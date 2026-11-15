#!/usr/bin/env bash
# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0
#
# Isolated testbed connectivity for the E2E harness (vmop-4430).
#
# On an isolated Nimbus testbed the vCenter, Supervisor, and ESXi hosts live on
# a private network that is only reachable through a single perimeter jumphost
# (the NAT gateway VM Nimbus attaches to the isolated network, which has a
# public address). The E2E runner cannot rely on privileged routing or sshuttle
# (it may run as an arbitrary, unprivileged UID with no CAP_NET_ADMIN), so
# connectivity is provided at the application layer:
#
#   * One authenticated SSH connection to the jumphost is kept open as a
#     ControlMaster. It also serves a local SOCKS5 listener (-D).
#   * Go clients, curl, and govc reach the private network via
#     ALL_PROXY=socks5h://127.0.0.1:<port>.
#   * ssh/scp to private hosts use an ssh_config whose ProxyCommand multiplexes
#     over the master connection (ssh -W), so the jumphost is only
#     authenticated once and no files outside the per-run state directory are
#     written.
#
# Everything is owned by a per-run state directory (a fresh mktemp -d, mode
# 0700, containing the control socket, supervisor pid and log, and ssh_config)
# and a dynamically allocated local port, so concurrent runs never overwrite
# each other's files or terminate each other's tunnels.
#
# This file is meant to be sourced by setup-testbed-env.sh, but it can also be
# executed to tear a tunnel down:
#
#   E2E_ISOLATED_STATE_DIR=/tmp/e2e-iso.XXXXXX ./hack/e2e/isolated.sh stop
#
# Environment overrides (all optional; inputs only, never written by setup):
#   E2E_ISOLATED          true|false  Force isolated mode on or off instead of
#                                     auto-detecting it from the testbed JSON.
#   E2E_JUMPHOST_IP       Jumphost address       (default: from testbed JSON).
#   E2E_JUMPHOST_USERNAME Jumphost SSH user      (default: from testbed JSON).
#   E2E_JUMPHOST_PASSWORD Jumphost SSH password  (default: from testbed JSON).
#
# Variables exported when isolated mode is active (and unset again when the
# tunnel is stopped or a later setup in the same shell replaces it):
#   E2E_ISOLATED_STATE_DIR, E2E_SSH_CONFIG, E2E_MGMT_CIDR, E2E_MGMT_IP_PREFIX,
#   ALL_PROXY/HTTPS_PROXY/HTTP_PROXY (and lowercase), NO_PROXY/no_proxy.

# Fallback helpers so the library works when executed on its own or sourced by
# a script (run-e2e.sh) that does not define them.
if ! declare -F _log >/dev/null 2>&1; then
    _log()  { printf '%s\n' "$*" >&2; }
    _warn() { printf 'Warning: %s\n' "$*" >&2; }
    _err()  { printf 'Error: %s\n' "$*" >&2; }
fi
if ! declare -F _export >/dev/null 2>&1; then
    _export() { export "$1=$2"; }
fi
if ! declare -F _unexport >/dev/null 2>&1; then
    _unexport() { unset "$1"; }
fi

# Every variable the tunnel exports. Unset when the tunnel goes away so that no
# later command (or later setup in the same shell) inherits a dead proxy port or
# a deleted ssh_config.
_ISO_EXPORTED_VARS="E2E_ISOLATED_STATE_DIR E2E_SSH_CONFIG E2E_MGMT_CIDR E2E_MGMT_IP_PREFIX ALL_PROXY all_proxy HTTPS_PROXY https_proxy HTTP_PROXY http_proxy NO_PROXY no_proxy"

# _iso_ip_to_int <a.b.c.d>: prints the address as a 32-bit integer.
_iso_ip_to_int() {
    local IFS=.
    # shellcheck disable=SC2206
    local -a o=($1)
    [[ ${#o[@]} -eq 4 ]] || return 1
    local x
    for x in "${o[@]}"; do
        [[ "${x}" =~ ^[0-9]+$ ]] && (( x <= 255 )) || return 1
    done
    printf '%s\n' $(( (o[0] << 24) | (o[1] << 16) | (o[2] << 8) | o[3] ))
}

# _iso_ip_in_cidr <ip> <a.b.c.d/n>: succeeds when <ip> lies inside the CIDR.
_iso_ip_in_cidr() {
    local ip net bits
    ip="$(_iso_ip_to_int "$1")" || return 1
    net="$(_iso_ip_to_int "${2%/*}")" || return 1
    bits="${2#*/}"
    [[ "${bits}" =~ ^[0-9]+$ ]] && (( bits <= 32 )) || return 1
    local mask=$(( bits == 0 ? 0 : (0xFFFFFFFF << (32 - bits)) & 0xFFFFFFFF ))
    (( (ip & mask) == (net & mask) ))
}

# _iso_prefix_to_cidr <prefix>
#
# Normalizes a network prefix to "<cidr> <grep-prefix>". Accepts the forms
# Nimbus and vdnet use: one to three dotted octets ("172.21", "172.21.1",
# optionally with a trailing dot) or a CIDR ("172.21.1.0/24"). The grep prefix
# is the dotted octets covered by the mask, with a trailing dot (e.g.
# "172.21.1."), for the grep-based fallback in proxy.sh/kms.sh.
_iso_prefix_to_cidr() {
    local p="${1%.}" cidr bits
    if [[ "${p}" == */* ]]; then
        cidr="${p}"
        bits="${p#*/}"
        _iso_ip_to_int "${p%/*}" >/dev/null || return 1
    else
        local IFS=.
        # shellcheck disable=SC2206
        local -a o=(${p})
        unset IFS
        (( ${#o[@]} >= 1 && ${#o[@]} <= 3 )) || return 1
        bits=$(( ${#o[@]} * 8 ))
        local net="${p}"
        local i
        for (( i = ${#o[@]}; i < 4; i++ )); do net+=".0"; done
        _iso_ip_to_int "${net}" >/dev/null || return 1
        cidr="${net}/${bits}"
    fi
    [[ "${bits}" =~ ^[0-9]+$ ]] && (( bits >= 8 && bits <= 32 )) || return 1
    local octets=$(( bits / 8 ))
    (( octets > 3 )) && octets=3
    printf '%s %s.\n' "${cidr}" "$(cut -d. -f1-"${octets}" <<< "${cidr%/*}")"
}

# _iso_network_prefix <testbed_json>: prints the isolated network prefix, if
# any: isolatedNetworkOptions.networkPrefix (top-level or inside testbed_spec),
# else the network_prefix of the management network entry (vdnet-style files).
_iso_network_prefix() {
    jq -r '
        ((.isolatedNetworkOptions // .testbed_spec.isolatedNetworkOptions // {}) | if type == "object" then .networkPrefix else null end)
        // ((.network // []) | (if type == "object" then [.[]] elif type == "array" then . else [] end)
            | map(select(type == "object" and .management == "yes" and (.network_prefix // "") != ""))
            | .[0].network_prefix)
        // empty' <<< "$1" 2>/dev/null
}

# _iso_jumphost_entry <testbed_json>: prints the jumphost network entry (a
# network[] entry with both a gateway address and a username) as compact JSON.
# network is an array in some testbed files and an index-keyed object in
# others; both are accepted.
_iso_jumphost_entry() {
    jq -c '
        (.network // []) | (if type == "object" then [.[]] elif type == "array" then . else [] end)
        | map(select(type == "object" and (.gateway // "") != "" and (.username // "") != ""))
        | .[0] // empty' <<< "$1" 2>/dev/null
}

# _iso_is_isolated <vc_ip> <testbed_json>...
#
# Succeeds when the testbed is isolated. E2E_ISOLATED=true|false wins.
# Otherwise either signal, in any of the given JSON documents, is sufficient:
#   - an isolated network prefix is declared AND the vCenter IP sits inside it.
#     The options block alone is not enough: Nimbus emits it on ordinary
#     testbeds too, where the VC has a routable address outside the prefix.
#   - the network list holds a jumphost entry (a gateway address plus a
#     username). Ordinary testbeds have none. Testbed files that come from
#     vdnet carry no isolatedNetworkOptions, so this is the only signal there.
# <vc_ip> is the vCenter address setup already parsed, so every vc shape that
# setup understands is covered.
_iso_is_isolated() {
    case "${E2E_ISOLATED:-}" in
        true|1)  return 0 ;;
        false|0) return 1 ;;
    esac
    local -r vc_ip="$1"
    shift
    local doc prefix net
    for doc in "$@"; do
        [[ -n "$(_iso_jumphost_entry "${doc}")" ]] && return 0
        prefix="$(_iso_network_prefix "${doc}")"
        [[ -n "${prefix}" ]] || continue
        net="$(_iso_prefix_to_cidr "${prefix}")" || continue
        _iso_ip_in_cidr "${vc_ip}" "${net%% *}" && return 0
    done
    return 1
}

# _iso_parse_jumphost <testbed_json>...
#
# Populates iso_jump_ip, iso_jump_user, iso_jump_password from the first
# document with a jumphost entry: the network[] entry that carries both a
# gateway address and a username (its vm_resource describes the NAT gateway VM
# that is the jumphost). This is deliberately distinct from the in-testbed
# "external gateway" VM (which fronts guest workloads and is discovered via
# govc). E2E_JUMPHOST_* override individual fields.
_iso_parse_jumphost() {
    iso_jump_ip="${E2E_JUMPHOST_IP:-}"
    iso_jump_user="${E2E_JUMPHOST_USERNAME:-}"
    iso_jump_password="${E2E_JUMPHOST_PASSWORD:-}"

    local doc entry=""
    for doc in "$@"; do
        entry="$(_iso_jumphost_entry "${doc}")"
        [[ -n "${entry}" ]] && break
    done

    if [[ -n "${entry}" ]]; then
        [[ -n "${iso_jump_ip}" ]]       || iso_jump_ip=$(jq -r '.gateway' <<< "${entry}")
        [[ -n "${iso_jump_user}" ]]     || iso_jump_user=$(jq -r '.username' <<< "${entry}")
        [[ -n "${iso_jump_password}" ]] || iso_jump_password=$(jq -r '.password // empty' <<< "${entry}")
    fi

    if [[ -z "${iso_jump_ip}" || -z "${iso_jump_user}" || -z "${iso_jump_password}" ]]; then
        _err "Testbed is isolated but no jumphost (address, username, password) was found in the testbed data."
        _err "Set E2E_JUMPHOST_IP, E2E_JUMPHOST_USERNAME and E2E_JUMPHOST_PASSWORD to override."
        return 1
    fi
    _log "Isolated testbed jumphost: ${iso_jump_user}@${iso_jump_ip}"
}

# _iso_mgmt_network <vc_ip> <testbed_json>...
#
# Prints "<ip-prefix> <cidr>" for the testbed's management network, used to
# pick the external gateway VM's management address: the declared isolated
# network prefix when there is one, otherwise the vCenter's /24.
_iso_mgmt_network() {
    local -r vc_ip="$1"
    shift
    local doc prefix net
    for doc in "$@"; do
        prefix="$(_iso_network_prefix "${doc}")"
        [[ -n "${prefix}" ]] || continue
        if net="$(_iso_prefix_to_cidr "${prefix}")"; then
            printf '%s %s\n' "${net##* }" "${net%% *}"
            return 0
        fi
        _warn "Ignoring unrecognized isolated network prefix \"${prefix}\""
    done
    net="$(_iso_prefix_to_cidr "$(cut -d. -f1-3 <<< "${vc_ip}")")" || net="10.0.0.0/8 10."
    printf '%s %s\n' "${net##* }" "${net%% *}"
}

# _iso_url_host <url>: prints the host part of a URL (empty if none).
_iso_url_host() {
    sed -nE 's#^[A-Za-z][A-Za-z0-9+.-]*://([^/@]*@)?(\[[^]]*\]|[^/:?#]*).*#\2#p' <<< "$1"
}

# _iso_probe_socks <port> <host>
#
# Verifies the tunnel end to end: a TLS connection to <host>:443 must succeed
# through the SOCKS listener. Any HTTP status is fine; 000 means no connection.
_iso_probe_socks() {
    local code
    code=$(curl -sk -m 10 -o /dev/null -w '%{http_code}' \
        --socks5-hostname "127.0.0.1:$1" "https://$2/" 2>/dev/null) || true
    [[ -n "${code}" && "${code}" != "000" ]]
}

# _iso_supervise <state_dir> <socks_port>
#
# Runs in the background for the life of the run. Keeps the jumphost master
# connection alive: when ssh exits (dropped link, server restart, kill), it is
# relaunched on the same control socket and SOCKS port, so the proxy variables
# and ssh_config that are already exported stay valid. Every forward recorded
# in <state_dir>/forwards is re-added once the new master is up; a forward that
# cannot be re-added (its local port was taken meanwhile) is logged and skipped
# rather than taking the SOCKS listener down with it. In-flight connections are
# lost across a reconnect; callers retry.
#
# Gives up (writing <state_dir>/failed) after 5 consecutive attempts that each
# lasted under a minute. That window is well above ConnectTimeout, so a bad
# password and an unreachable jumphost both count, and neither can spin
# forever or hammer the jumphost with logins once the testbed is gone.
_iso_supervise() {
    local -r dir="$1" port="$2"
    local short_runs=0 started line rc ssh_pid i
    while :; do
        # A killed master leaves its socket behind; a new -M would silently
        # run without multiplexing if the file still exists.
        rm -f "${dir}/ctl"

        started=${SECONDS}
        SSHPASS="${iso_jump_password}" sshpass -e ssh -M -S "${dir}/ctl" -N \
            -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
            -o PubkeyAuthentication=no -o PreferredAuthentications=password \
            -o ConnectTimeout=20 -o LogLevel=ERROR \
            -o ExitOnForwardFailure=yes \
            -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
            -D "127.0.0.1:${port}" \
            "${iso_jump_user}@${iso_jump_ip}" &
        ssh_pid=$!

        # Re-add recorded forwards once the master answers on its socket.
        for (( i = 0; i < 40; i++ )); do
            kill -0 "${ssh_pid}" 2>/dev/null || break
            if ssh -S "${dir}/ctl" -O check "${iso_jump_user}@${iso_jump_ip}" >/dev/null 2>&1; then
                while IFS= read -r line; do
                    [[ -n "${line}" ]] || continue
                    ssh -S "${dir}/ctl" -O forward -L "${line}" "${iso_jump_user}@${iso_jump_ip}" >/dev/null 2>&1 \
                        || printf '%s could not re-add forward %s\n' "$(date -u +%FT%TZ)" "${line}"
                done < "${dir}/forwards"
                break
            fi
            sleep 1
        done

        rc=0
        wait "${ssh_pid}" || rc=$?

        if (( SECONDS - started < 60 )); then
            short_runs=$(( short_runs + 1 ))
        else
            short_runs=0
        fi
        printf '%s jumphost tunnel exited (rc=%s, %s consecutive short runs)\n' \
            "$(date -u +%FT%TZ)" "${rc}" "${short_runs}"
        if (( short_runs >= 5 )); then
            echo "giving up on the jumphost tunnel" > "${dir}/failed"
            return 1
        fi
        sleep 2
    done
}

# _iso_jump_target <state_dir>: prints the jumphost user@address recorded for
# the tunnel. Control-socket commands (-O check/forward/cancel/exit) only use
# the socket, but still need a destination argument.
_iso_jump_target() {
    cat "$1/jump" 2>/dev/null || printf 'jumphost\n'
}

# _iso_kill_tunnel_procs <state_dir>
#
# Stops the supervisor loop first, so it cannot relaunch the master, then every
# process whose command line names this run's control socket: sshpass, the
# master ssh (a grandchild of the supervisor, so a parent-pid match cannot
# reach it), ProxyCommand hops, and any of those re-parented to init. The state
# directory path is a unique mktemp name, so nothing else can match.
_iso_kill_tunnel_procs() {
    local -r dir="$1"
    local pid
    pid="$(cat "${dir}/supervisor.pid" 2>/dev/null || true)"
    if [[ "${pid}" =~ ^[0-9]+$ ]]; then
        kill -TERM "${pid}" 2>/dev/null || true
    fi
    rm -f "${dir}/supervisor.pid"
    pkill -TERM -f -- "${dir}/ctl" 2>/dev/null || true
}

# _iso_start_tunnel <vc_ip> [url...]
#
# Starts the jumphost ControlMaster + SOCKS listener and writes the ssh_config.
# Reads iso_jump_*. The hosts of the given URLs (UTS callback, testbed data)
# are exempted from the proxy, since the runner reaches them directly. Fails
# (returning 1, after cleaning up) if the tunnel cannot be established or cannot
# reach the vCenter, so setup stops with a useful message instead of timing out
# later.
_iso_start_tunnel() {
    local -r vc_ip="$1"
    shift

    local state_dir
    # /tmp (not $TMPDIR): control socket paths are limited to ~100 characters.
    if ! state_dir="$(mktemp -d /tmp/e2e-iso.XXXXXX)"; then
        _err "Failed to create the isolated-connectivity state directory under /tmp"
        return 1
    fi
    chmod 700 "${state_dir}"

    local -r ctl="${state_dir}/ctl"
    : > "${state_dir}/forwards"
    printf '%s@%s\n' "${iso_jump_user}" "${iso_jump_ip}" > "${state_dir}/jump"

    # Pick a random local port and let ssh claim it (ExitOnForwardFailure makes
    # a collision fail loudly), retrying on a port clash only. Pure bash so the
    # runner image needs no python. The master is run by a supervisor that
    # restarts it if the connection drops (see _iso_supervise).
    local port attempt i started=false
    for attempt in 1 2 3; do
        port=$(( 20000 + RANDOM % 40000 ))
        _log "Starting jumphost tunnel (SOCKS5 on 127.0.0.1:${port})..."
        : > "${state_dir}/supervisor.log"
        rm -f "${ctl}" "${state_dir}/failed"
        (
            # Setup may run under "set -e" (when executed rather than sourced);
            # the supervisor must survive its ssh child exiting non-zero.
            set +e
            trap '' HUP
            _iso_supervise "${state_dir}" "${port}"
        ) </dev/null >>"${state_dir}/supervisor.log" 2>&1 &
        echo $! > "${state_dir}/supervisor.pid"
        disown 2>/dev/null || true

        # Wait for the master to come up, or for the supervisor to give up.
        for i in $(seq 1 40); do
            ssh -S "${ctl}" -O check "${iso_jump_user}@${iso_jump_ip}" >/dev/null 2>&1 && { started=true; break; }
            [[ -e "${state_dir}/failed" ]] && break
            sleep 1
        done
        [[ "${started}" == "true" ]] && break

        # Not up: stop this supervisor. Retry only on a local port clash.
        _iso_kill_tunnel_procs "${state_dir}"
        grep -qiE 'address already in use|cannot listen|forwarding failed|bind' "${state_dir}/supervisor.log" || break
    done
    if [[ "${started}" != "true" ]]; then
        _err "Could not open an SSH connection to the jumphost ${iso_jump_user}@${iso_jump_ip}: $(tail -n 3 "${state_dir}/supervisor.log" | tr '\n' ' ')"
        rm -rf "${state_dir}"
        return 1
    fi

    # Export the state dir immediately so a later failure can still tear down.
    _export E2E_ISOLATED_STATE_DIR "${state_dir}"

    local ready=false
    for i in 1 2 3 4 5; do
        if _iso_probe_socks "${port}" "${vc_ip}"; then ready=true; break; fi
        sleep 2
    done
    if [[ "${ready}" != "true" ]]; then
        _err "Jumphost tunnel is up but cannot reach vCenter ${vc_ip}:443 through it (is ${iso_jump_ip} routed to the isolated network?)"
        _iso_stop_tunnel
        return 1
    fi

    # ssh/scp to private hosts hop through the already-authenticated master.
    cat > "${state_dir}/ssh_config" <<SSHCFG
Host * !${iso_jump_ip}
    ProxyCommand ssh -S ${ctl} -o BatchMode=yes -o LogLevel=ERROR -W %h:%p ${iso_jump_user}@${iso_jump_ip}
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
SSHCFG

    local socks="socks5h://127.0.0.1:${port}"
    # Internal Broadcom/VMware services (UTS callback and log hosts, package
    # mirrors) and the hosts of the given URLs are reachable from the runner
    # directly and must not be sent to the jumphost. The private testbed
    # addresses are deliberately not listed.
    local no_proxy="localhost,127.0.0.1,.broadcom.net,.vmware.com" u h
    for u in "$@"; do
        h="$(_iso_url_host "${u}")"
        h="${h#[}"; h="${h%]}"
        [[ -n "${h}" ]] && no_proxy+=",${h}"
    done
    _export E2E_SSH_CONFIG "${state_dir}/ssh_config"
    local v
    for v in ALL_PROXY all_proxy HTTPS_PROXY https_proxy HTTP_PROXY http_proxy; do
        _export "${v}" "${socks}"
    done
    _export NO_PROXY "${no_proxy}"
    _export no_proxy "${no_proxy}"
    _log "✓ Jumphost tunnel ready: ALL_PROXY=${socks} NO_PROXY=${no_proxy} (state: ${state_dir})"
}

# _iso_add_http_proxy <host> <port> <probe_host>
#
# Adds an HTTP CONNECT proxy for HTTP clients on top of the SOCKS tunnel.
#
# SOCKS is enough for REST (govc, curl, client-go requests) but not for
# Kubernetes streaming: kubectl/client-go reject a socks5(h):// proxy for
# exec/attach/port-forward (WebSocket: "unknown scheme"; SPDY: "proxy URL scheme
# not supported"). A local port forward on the existing master to a real HTTP
# proxy (the Squid that setup installs on the external gateway, which accepts
# CONNECT to any port) fixes this without a second SSH login.
#
# On success HTTP(S)_PROXY point at the local forward; ALL_PROXY stays SOCKS for
# the Go SSH dialer. If the forward cannot be verified the SOCKS settings are
# left in place and a warning says streaming will not work.
_iso_add_http_proxy() {
    local -r host="$1" port="$2" probe_host="$3"
    local -r dir="${E2E_ISOLATED_STATE_DIR:-}"
    [[ -n "${dir}" && -S "${dir}/ctl" ]] || return 1
    local target
    target="$(_iso_jump_target "${dir}")"

    local lp attempt out ok=false
    for attempt in 1 2 3 4 5; do
        lp=$(( 20000 + RANDOM % 40000 ))
        if out=$(ssh -S "${dir}/ctl" -O forward -L "127.0.0.1:${lp}:${host}:${port}" "${target}" 2>&1); then
            ok=true
            break
        fi
        grep -qiE 'address already in use|cannot listen|forwarding failed|bind' <<< "${out}" || break
    done
    if [[ "${ok}" != "true" ]]; then
        _warn "Could not forward a local port to the HTTP proxy ${host}:${port}: ${out}"
        return 1
    fi

    local code
    code=$(curl -sk -m 15 -o /dev/null -w '%{http_code}' \
        --proxy "http://127.0.0.1:${lp}" "https://${probe_host}/" 2>/dev/null) || true
    if [[ -z "${code}" || "${code}" == "000" ]]; then
        ssh -S "${dir}/ctl" -O cancel -L "127.0.0.1:${lp}:${host}:${port}" "${target}" >/dev/null 2>&1 || true
        _warn "HTTP proxy ${host}:${port} is not usable through the tunnel"
        return 1
    fi

    # Remember the forward so the supervisor re-establishes it after a reconnect.
    echo "127.0.0.1:${lp}:${host}:${port}" >> "${dir}/forwards"

    local v
    for v in HTTPS_PROXY https_proxy HTTP_PROXY http_proxy; do
        _export "${v}" "http://127.0.0.1:${lp}"
    done
    _log "✓ HTTP CONNECT proxy for Kubernetes streaming: http://127.0.0.1:${lp} -> ${host}:${port}"
}

# _iso_stop_tunnel
#
# Closes the tunnel owned by E2E_ISOLATED_STATE_DIR, removes the state
# directory, and unsets every variable the tunnel exported, so nothing is left
# pointing at a dead local port or a deleted ssh_config. Idempotent; never
# touches another run's tunnel. A no-op when no tunnel was recorded.
_iso_stop_tunnel() {
    local -r dir="${E2E_ISOLATED_STATE_DIR:-}"
    [[ -n "${dir}" ]] || return 0

    if [[ -d "${dir}" && "${dir}" == /tmp/e2e-iso.* ]]; then
        _iso_kill_tunnel_procs "${dir}"
        if [[ -S "${dir}/ctl" ]]; then
            ssh -S "${dir}/ctl" -O exit "$(_iso_jump_target "${dir}")" >/dev/null 2>&1 || true
        fi
        rm -rf "${dir}"
        _log "Jumphost tunnel stopped (${dir})"
    fi

    local v
    for v in ${_ISO_EXPORTED_VARS}; do
        _unexport "${v}"
    done
}

# Executed (not sourced): support "stop".
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
    case "${1:-}" in
        stop) _iso_stop_tunnel ;;
        *)    echo "Usage: E2E_ISOLATED_STATE_DIR=<dir> $0 stop" >&2; exit 1 ;;
    esac
fi
