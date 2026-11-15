#!/usr/bin/env bash
# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0
#
# Isolated testbed connectivity for the E2E harness (vmop-4430).
#
# On an isolated Nimbus testbed the vCenter, Supervisor, and ESXi hosts live on
# a private network that is only reachable through a single perimeter jumphost
# (a "worker" VM with a public address). The E2E runner cannot rely on
# privileged routing or sshuttle (it may run as an arbitrary, unprivileged UID
# with no CAP_NET_ADMIN), so connectivity is provided at the application layer:
#
#   * One authenticated SSH connection to the jumphost is kept open as a
#     ControlMaster. It also serves a local SOCKS5 listener (-D).
#   * Go clients, curl, and govc reach the private network via
#     ALL_PROXY / HTTPS_PROXY=socks5h://127.0.0.1:<port>.
#   * ssh/scp to private hosts use an ssh_config whose ProxyCommand multiplexes
#     over the master connection (ssh -W), so the jumphost is only
#     authenticated once and no files outside the per-run state directory are
#     written.
#
# Everything is owned by a per-run state directory (a fresh mktemp -d, mode
# 0700, containing the control socket, pid file, and ssh_config) and a
# dynamically allocated local port, so concurrent runs never overwrite each
# other's files or terminate each other's tunnels.
#
# This file is meant to be sourced by setup-testbed-env.sh, but it can also be
# executed to tear a tunnel down:
#
#   E2E_ISOLATED_STATE_DIR=/tmp/e2e-iso.XXXXXX ./hack/e2e/isolated.sh stop
#
# Environment overrides (all optional):
#   E2E_ISOLATED          true|false  Force isolated mode on or off instead of
#                                     auto-detecting it from the testbed JSON.
#   E2E_JUMPHOST_IP       Jumphost address       (default: from testbed JSON).
#   E2E_JUMPHOST_USERNAME Jumphost SSH user      (default: from testbed JSON).
#   E2E_JUMPHOST_PASSWORD Jumphost SSH password  (default: from testbed JSON).
#
# Variables exported when isolated mode is active:
#   E2E_ISOLATED=true, E2E_ISOLATED_STATE_DIR, E2E_SSH_CONFIG,
#   E2E_JUMPHOST_IP/USERNAME/PASSWORD, E2E_MGMT_CIDR, E2E_MGMT_IP_PREFIX,
#   ALL_PROXY/HTTPS_PROXY/HTTP_PROXY (and lowercase), NO_PROXY/no_proxy.

# Fallback loggers so the library works when executed on its own.
if ! declare -F _log >/dev/null 2>&1; then
    _log()  { printf '%s\n' "$*" >&2; }
    _warn() { printf 'Warning: %s\n' "$*" >&2; }
    _err()  { printf 'Error: %s\n' "$*" >&2; }
fi

# _iso_is_isolated <testbed_info_json>
#
# Succeeds when the testbed is isolated. E2E_ISOLATED=true|false wins.
# Otherwise isolatedNetworkOptions.networkPrefix (top-level or inside
# testbed_spec) must be present AND the vCenter IP must sit inside that
# prefix. The options block alone is not enough: Nimbus emits it on ordinary
# testbeds too, where the VC has a routable address outside the prefix.
_iso_is_isolated() {
    case "${E2E_ISOLATED:-}" in
        true|1)  return 0 ;;
        false|0) return 1 ;;
    esac
    jq -e '
        (.isolatedNetworkOptions // .testbed_spec.isolatedNetworkOptions // {}).networkPrefix as $p
        | ((.vc | if type == "array" then .[0] else . end) | if type == "object" then .ip else . end // "") as $vc
        | ($p != null and ($vc | type) == "string" and ($vc | startswith($p + ".")))
    ' <<< "$1" >/dev/null 2>&1
}

# _iso_parse_jumphost <testbed_info_json>
#
# Populates iso_jump_ip, iso_jump_user, iso_jump_password.
#
# The jumphost is the NAT gateway Nimbus attaches to the isolated network: the
# top-level network[] entry that carries both a gateway address and a username
# (its vm_resource describes the gateway VM). Every VM entry's own "gateway"
# field points at the same address, so worker[0] is used as a fallback. This is
# deliberately distinct from the in-testbed "external gateway" VM (which fronts
# guest workloads and is discovered via govc).
_iso_parse_jumphost() {
    local -r info="$1"
    iso_jump_ip="${E2E_JUMPHOST_IP:-}"
    iso_jump_user="${E2E_JUMPHOST_USERNAME:-}"
    iso_jump_password="${E2E_JUMPHOST_PASSWORD:-}"

    local entry
    entry=$(jq -c '
        ((.network // []) | map(select((.gateway // "") != "" and (.username // "") != "")) | .[0]) //
        ((.worker // []) | map(select((.gateway // "") != "" and (.username // "") != "")) | .[0]) //
        empty' <<< "${info}" 2>/dev/null)

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

# _iso_mgmt_network <testbed_info_json> <vc_ip>
#
# Prints "<ip-prefix> <cidr>" for the testbed's management network, used to
# pick the external gateway VM's management address. Isolated testbeds declare
# networkPrefix (e.g. "172.21.1"); otherwise the vCenter's /24 is assumed.
_iso_mgmt_network() {
    local prefix
    prefix=$(jq -r '(.isolatedNetworkOptions // .testbed_spec.isolatedNetworkOptions // {}).networkPrefix // empty' <<< "$1" 2>/dev/null)
    [[ -n "${prefix}" ]] || prefix="$(cut -d. -f1-3 <<< "$2")"
    printf '%s. %s.0/24\n' "${prefix}" "${prefix}"
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
# relaunched on the same control socket and SOCKS port, with every forward
# recorded in <state_dir>/forwards, so the proxy variables and ssh_config that
# are already exported stay valid. In-flight connections are lost across a
# reconnect; callers retry.
#
# Gives up (writing <state_dir>/failed) after 5 consecutive failures that each
# ended within 15 seconds of starting, so a bad password or an unreachable
# jumphost cannot spin forever or hammer the jumphost with logins.
_iso_supervise() {
    local -r dir="$1" port="$2"
    local fast_failures=0 started args line
    while :; do
        # A killed master leaves its socket behind; a new -M would silently
        # run without multiplexing if the file still exists.
        rm -f "${dir}/ctl"
        args=()
        while IFS= read -r line; do
            [[ -n "${line}" ]] && args+=(-L "${line}")
        done < "${dir}/forwards"

        started=${SECONDS}
        SSHPASS="${iso_jump_password}" sshpass -e ssh -M -S "${dir}/ctl" -N \
            -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
            -o PubkeyAuthentication=no -o PreferredAuthentications=password \
            -o ConnectTimeout=20 -o LogLevel=ERROR \
            -o ExitOnForwardFailure=yes \
            -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
            -D "127.0.0.1:${port}" "${args[@]}" \
            "${iso_jump_user}@${iso_jump_ip}"
        local rc=$?

        if (( SECONDS - started < 15 )); then
            fast_failures=$(( fast_failures + 1 ))
        else
            fast_failures=0
        fi
        printf '%s jumphost tunnel exited (rc=%s, %s consecutive fast failures); restarting\n' \
            "$(date -u +%FT%TZ)" "${rc}" "${fast_failures}"
        if (( fast_failures >= 5 )); then
            echo "giving up on the jumphost tunnel" > "${dir}/failed"
            return 1
        fi
        sleep 2
    done
}

# _iso_kill_supervisor <state_dir>: stops the supervisor loop and its ssh child.
_iso_kill_supervisor() {
    local -r dir="$1"
    local pid
    pid="$(cat "${dir}/supervisor.pid" 2>/dev/null || true)"
    [[ "${pid}" =~ ^[0-9]+$ ]] || return 0
    # Kill the loop first so it cannot relaunch, then its ssh child.
    kill -TERM "${pid}" 2>/dev/null || true
    pkill -TERM -P "${pid}" 2>/dev/null || true
    rm -f "${dir}/supervisor.pid"
}

# _iso_start_tunnel <vc_ip>
#
# Starts the jumphost ControlMaster + SOCKS listener and writes the ssh_config.
# Reads iso_jump_*. Fails (returning 1, after cleaning up) if the tunnel cannot
# be established or cannot reach the vCenter, so setup stops with a useful
# message instead of timing out later.
_iso_start_tunnel() {
    local -r vc_ip="$1"

    local state_dir
    # /tmp (not $TMPDIR): control socket paths are limited to ~100 characters.
    if ! state_dir="$(mktemp -d /tmp/e2e-iso.XXXXXX)"; then
        _err "Failed to create the isolated-connectivity state directory under /tmp"
        return 1
    fi
    chmod 700 "${state_dir}"

    local -r ctl="${state_dir}/ctl"
    : > "${state_dir}/forwards"

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
        _iso_kill_supervisor "${state_dir}"
        grep -qiE 'address already in use|cannot listen|forwarding failed|bind' "${state_dir}/supervisor.log" || break
    done
    if [[ "${started}" != "true" ]]; then
        _err "Could not open an SSH connection to the jumphost ${iso_jump_user}@${iso_jump_ip}: $(tail -n 3 "${state_dir}/supervisor.log" | tr '\n' ' ')"
        rm -rf "${state_dir}"
        return 1
    fi

    # Export the state dir immediately so a later failure can still tear down.
    _export E2E_ISOLATED_STATE_DIR "${state_dir}"
    _export E2E_JUMPHOST_IP        "${iso_jump_ip}"
    _export E2E_JUMPHOST_USERNAME  "${iso_jump_user}"

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
    cat > "${state_dir}/ssh_config" <<EOF
Host * !${iso_jump_ip}
    ProxyCommand ssh -S ${ctl} -o BatchMode=yes -o LogLevel=ERROR -W %h:%p ${iso_jump_user}@${iso_jump_ip}
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
EOF

    local socks="socks5h://127.0.0.1:${port}"
    # Internal Broadcom/VMware services (UTS callback and log hosts, package
    # mirrors) are reachable from the runner directly and must not be sent to
    # the jumphost. The private testbed addresses are deliberately not listed.
    local no_proxy="localhost,127.0.0.1,.broadcom.net,.vmware.com"
    _export E2E_ISOLATED             "true"
    _export E2E_SSH_CONFIG           "${state_dir}/ssh_config"
    _export E2E_JUMPHOST_IP          "${iso_jump_ip}"
    _export E2E_JUMPHOST_USERNAME    "${iso_jump_user}"
    _export E2E_JUMPHOST_PASSWORD    "${iso_jump_password}"
    local v
    for v in ALL_PROXY all_proxy HTTPS_PROXY https_proxy HTTP_PROXY http_proxy; do
        _export "${v}" "${socks}"
    done
    _export NO_PROXY "${no_proxy}"
    _export no_proxy "${no_proxy}"
    _log "✓ Jumphost tunnel ready: ALL_PROXY=${socks} (state: ${state_dir})"
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

    local lp attempt out ok=false
    for attempt in 1 2 3 4 5; do
        lp=$(( 20000 + RANDOM % 40000 ))
        if out=$(ssh -S "${dir}/ctl" -O forward -L "127.0.0.1:${lp}:${host}:${port}" \
                "${E2E_JUMPHOST_USERNAME}@${E2E_JUMPHOST_IP}" 2>&1); then
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
        ssh -S "${dir}/ctl" -O cancel -L "127.0.0.1:${lp}:${host}:${port}" \
            "${E2E_JUMPHOST_USERNAME}@${E2E_JUMPHOST_IP}" >/dev/null 2>&1 || true
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

# _iso_stop_tunnel: closes the tunnel owned by E2E_ISOLATED_STATE_DIR and
# removes the state directory. Idempotent; never touches anything else.
_iso_stop_tunnel() {
    local -r dir="${E2E_ISOLATED_STATE_DIR:-}"
    [[ -n "${dir}" && -d "${dir}" && "${dir}" == /tmp/e2e-iso.* ]] || return 0

    # Stop the supervisor first so it does not relaunch the master we close.
    _iso_kill_supervisor "${dir}"
    if [[ -S "${dir}/ctl" ]]; then
        ssh -S "${dir}/ctl" -O exit "${E2E_JUMPHOST_USERNAME:-x}@${E2E_JUMPHOST_IP:-x}" >/dev/null 2>&1 || true
    fi
    # Fall back to the recorded pid, but only if it is still one of our ssh
    # processes, so a recycled pid is never signalled.
    local pid
    pid="$(cat "${dir}/pid" 2>/dev/null || true)"
    if [[ "${pid}" =~ ^[0-9]+$ ]] && kill -0 "${pid}" 2>/dev/null \
            && ps -p "${pid}" -o args= 2>/dev/null | grep -qF -- "${dir}/ctl"; then
        kill -TERM "${pid}" 2>/dev/null || true
    fi
    rm -rf "${dir}"
    _log "Jumphost tunnel stopped (${dir})"
}

# Executed (not sourced): support "stop".
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
    case "${1:-}" in
        stop) _export() { :; }; _iso_stop_tunnel ;;
        *)    echo "Usage: E2E_ISOLATED_STATE_DIR=<dir> $0 stop" >&2; exit 1 ;;
    esac
fi
