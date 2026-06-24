#!/usr/bin/env bash
#
# Setup testbed environment for vm-operator E2E tests.
#
# When executed directly, prints "export VAR=value" lines to stdout so the
# caller can copy/paste them or apply them all at once with eval:
#
#   eval "$(./hack/e2e/setup-testbed-env.sh ./testbedinfo.json)"
#
# When sourced, variables are set directly in the caller's shell without
# contaminating it with strict-mode shell options (set -u, set -e, etc.).
#
# Usage:
#   # Direct execution — prints export lines to stdout, progress to stderr:
#   ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json
#   eval "$(./hack/e2e/setup-testbed-env.sh ./testbedinfo.json)"
#   eval "$(./hack/e2e/setup-testbed-env.sh https://example.com/testbed.json)"
#
#   # Source into the current shell (sets variables directly):
#   source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json
#
#   # Enable E2E kubeconfig setup, HTTP proxy, and KMS key providers:
#   source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json --e2e
#   eval "$(./hack/e2e/setup-testbed-env.sh ./testbedinfo.json --e2e)"
#

# ---------------------------------------------------------------------------
# Source detection.
# The shebang always runs the script as bash, so BASH_SOURCE[0] == $0 when
# executed directly.  When sourced from bash or zsh the two values diverge
# (or BASH_SOURCE[0] is unset in zsh).
# ---------------------------------------------------------------------------
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
    _SOURCED=false
    # Strict mode is safe inside a subprocess; never enable it when sourced
    # because set -u and set -e would contaminate the caller's shell.
    set -euo pipefail
else
    _SOURCED=true
fi

# ---------------------------------------------------------------------------
# Retry configuration.
# ---------------------------------------------------------------------------
# SSH to vCenter (decryptK8Pwd.py): Supervisor may still be initialising when
# the e2e runner starts, so allow a generous window.
_DECRYPT_RETRY_ATTEMPTS=6
_DECRYPT_RETRY_INITIAL_DELAY=10   # seconds; doubles each attempt (~5 min total)

# SCP kubeconfig from Supervisor control-plane VM.
_KUBECONFIG_RETRY_ATTEMPTS=5
_KUBECONFIG_RETRY_INITIAL_DELAY=5 # seconds; doubles each attempt (~2.5 min total)

# kubectl-vsphere plugin download + unzip (Supervisor may serve a 503 or return
# a truncated zip during cluster initialisation).
_KUBECTL_VSPHERE_RETRY_ATTEMPTS=5
_KUBECTL_VSPHERE_RETRY_INITIAL_DELAY=5 # seconds; doubles each attempt (~2.5 min total)

# ---------------------------------------------------------------------------
# Module-level helpers.
# All progress/diagnostic output goes to stderr so stdout stays clean for the
# "export VAR=value" lines (which the caller can pipe through eval).
# ---------------------------------------------------------------------------
_log()  { printf '%s\n'          "$*" >&2; }
_warn() { printf 'Warning: %s\n' "$*" >&2; }
_err()  { printf 'Error: %s\n'   "$*" >&2; }

# _retry_with_backoff <max_attempts> <initial_delay_seconds> <description> <cmd> [args...]
#
# Runs <cmd> [args...] up to <max_attempts> times with exponential backoff
# starting at <initial_delay_seconds>.  Returns 0 on the first success, or 1
# after all attempts are exhausted.
_retry_with_backoff() {
    local -r max_attempts="$1"
    local -r initial_delay="$2"
    local -r description="$3"
    shift 3

    local attempt=1
    local delay="${initial_delay}"
    while (( attempt <= max_attempts )); do
        if "$@"; then
            return 0
        fi
        if (( attempt < max_attempts )); then
            _warn "${description} failed (attempt ${attempt}/${max_attempts}); retrying in ${delay}s..."
            sleep "${delay}"
            delay=$(( delay * 2 ))
        fi
        (( attempt++ ))
    done
    _warn "${description} failed after ${max_attempts} attempts"
    return 1
}

# Export a variable and, when running as a subprocess, also emit an eval-safe
# "export VAR=value" line to stdout.
_export() {
    local -r _n="$1" _v="$2"
    export "${_n}=${_v}"
    if [[ "${_SOURCED}" == "false" ]]; then
        printf 'export %s=%q\n' "${_n}" "${_v}"
    fi
}

# Common SSH options shared across all sshpass/ssh/scp invocations.
_SSH_OPTS=(
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -o PubkeyAuthentication=no
    -o PreferredAuthentications=password
)

# ---------------------------------------------------------------------------
# _load_testbed <source>
#
# Fetches or reads the testbed JSON, detects format (UTS deliverable_blob vs.
# direct), and populates:
#   testbed_data   — raw JSON as fetched
#   testbed_info   — the vc/networking blob (unwrapped from deliverable_blob
#                    when present, otherwise identical to testbed_data)
#
# Also defines _jq, a convenience wrapper that runs jq against testbed_info.
# It relies on dynamic scoping: testbed_info must be declared local in _main
# so that _jq can read it from any function in the call chain.
# ---------------------------------------------------------------------------
_load_testbed() {
    local -r source="$1"

    if [[ "${source}" =~ ^https?:// ]]; then
        _log "Fetching testbed data from: ${source}"
        if ! testbed_data=$(curl -fsSL "${source}" 2>/dev/null); then
            _err "Failed to fetch testbed data from ${source}"
            return 1
        fi
        _log "Fetched testbed data"
    else
        _log "Loading testbed file: ${source}"
        if [[ ! -f "${source}" ]]; then
            _err "File not found: ${source}"
            return 1
        fi
        testbed_data=$(< "${source}")
        _log "Loaded testbed data"
    fi

    if jq -e '.deliverable_blob' <<< "${testbed_data}" >/dev/null 2>&1; then
        _log "Detected UTS deliverable_blob format"
        testbed_info=$(jq -r '.deliverable_blob' <<< "${testbed_data}")
    else
        _log "Detected direct testbed format"
        testbed_info="${testbed_data}"
    fi

    _jq() { jq -r "$@" <<< "${testbed_info}"; }
}

# ---------------------------------------------------------------------------
# _parse_vc_credentials
#
# Reads testbed_info (via _jq) and populates:
#   vc_url, vc_root_password, vc_root_old_password,
#   vc_root_username, vc_vim_username, vc_vim_password
#
# Supports four JSON shapes in priority order:
#   1. .vc[] array  (VDS testbeds)
#   2. .vc{}  object (VPC testbeds)
#   3. .vcenter_* fields
#   4. Simple/fallback fields
# ---------------------------------------------------------------------------
_parse_vc_credentials() {
    if jq -e '.vc | type == "array" and length > 0' <<< "${testbed_info}" >/dev/null 2>&1; then
        _log "vCenter format: vc[] array"
        vc_url=$(_jq '.vc[0].ip4 // .vc[0].ip // .vc[0].vcenter_ip')
        vc_root_password=$(_jq '.vc[0].root_password // .vc[0].password // .vc[0].vcenter_password')
        vc_root_old_password=$(_jq '.vc[0].old_password // ""')
        vc_root_username=$(_jq 'if .vc[0].root_password then "root" else (.vc[0].username // .vc[0].vcenter_username // "administrator@vsphere.local") end')
        # vimUsername/vimPassword are the vSphere API (govc) credentials —
        # distinct from the SSH root credentials in username/password.
        vc_vim_username=$(_jq '.vc[0].vimUsername // .vc[0].username // "administrator@vsphere.local"')
        vc_vim_password=$(_jq '.vc[0].vimPassword // .vc[0].password')

    elif jq -e '.vc | type == "object" and length > 0' <<< "${testbed_info}" >/dev/null 2>&1; then
        _log "vCenter format: vc{} object"
        vc_url=$(_jq '.vc | to_entries | .[0].value | .ip4 // .ip // .vcenter_ip')
        vc_root_password=$(_jq '.vc | to_entries | .[0].value | .root_password // .password // .vcenter_password')
        vc_root_old_password=$(_jq '.vc | to_entries | .[0].value | .old_password // ""')
        vc_root_username=$(_jq '.vc | to_entries | .[0].value | if .root_password then "root" else (.username // .vcenter_username // "administrator@vsphere.local") end')
        vc_vim_username=$(_jq '.vc | to_entries | .[0].value | .vimUsername // .username // "administrator@vsphere.local"')
        vc_vim_password=$(_jq '.vc | to_entries | .[0].value | .vimPassword // .password')

    elif jq -e '.vcenter_ip' <<< "${testbed_info}" >/dev/null 2>&1; then
        _log "vCenter format: vcenter_* fields"
        vc_url=$(_jq '.vcenter_ip')
        vc_root_password=$(_jq '.vcenter_password')
        vc_root_old_password=$(_jq '.vcenter_old_password // ""')
        vc_root_username=$(_jq '.vcenter_username // "administrator@vsphere.local"')
        vc_vim_username="${vc_root_username}"
        vc_vim_password="${vc_root_password}"

    else
        _log "vCenter format: simple/fallback fields"
        vc_url=$(_jq '.vc_ip // .vcenter_ip // .testbed_ip')
        vc_root_password=$(_jq '.vc_password // .vcenter_password // .password')
        vc_root_old_password=""
        vc_root_username=$(_jq '.vc_username // .vcenter_username // .username // "administrator@vsphere.local"')
        vc_vim_username="${vc_root_username}"
        vc_vim_password="${vc_root_password}"
    fi

    if [[ -z "${vc_url}" || "${vc_url}" == "null" ]]; then
        _err "Could not extract vCenter IP from testbed data."
        _err "Top-level keys: $(jq -r 'keys | join(", ")' <<< "${testbed_info}" 2>/dev/null || echo '(invalid JSON)')"
        return 1
    fi
    if [[ -z "${vc_root_password}" || "${vc_root_password}" == "null" ]]; then
        _err "Could not extract vCenter root password from testbed data."
        return 1
    fi
    [[ -z "${vc_root_username}" || "${vc_root_username}" == "null" ]] && vc_root_username="root"

    _log "vCenter: ${vc_url}  (SSH: ${vc_root_username}, vSphere API: ${vc_vim_username})"
}

# ---------------------------------------------------------------------------
# _export_common_vars
#
# Exports all standard test-framework environment variables derived from the
# parsed vCenter credentials and networking type.
# Reads: vc_*, testbed_data (via dynamic scoping from _main).
# ---------------------------------------------------------------------------
_export_common_vars() {
    _export VC_URL               "${vc_url}"
    _export VC_ROOT_PASSWORD     "${vc_root_password}"
    _export VC_ROOT_OLD_PASSWORD "${vc_root_old_password}"
    _export VC_ROOT_USERNAME     "${vc_root_username}"
    _export VC_VIM_USERNAME      "${vc_vim_username}"
    _export VC_VIM_PASSWORD      "${vc_vim_password}"

    # Aliases expected by various test helpers and govc.
    _export VCSA_IP       "${vc_url}"
    _export SSH_USERNAME  "${vc_root_username}"
    _export SSH_PASSWORD  "${vc_root_password}"
    _export GOVC_USERNAME "${vc_vim_username}"
    _export GOVC_PASSWORD "${vc_vim_password}"
    _export VCSA_PASSWORD "${vc_vim_password}"

    local networking_type
    networking_type=$(jq -r '
        .deliverable_blob.networking //
        .networking //
        (if .TESTBED_TOPOLOGY == "NIMBUS_NSXT" then "nsx" else "vds" end)
    ' <<< "${testbed_data}")
    _export NETWORK "${networking_type}"

    # Honour values already set in the environment; fall back to safe defaults.
    _export TEST_SKIP  "${TEST_SKIP:-}"
    _export TEST_TAGS  "${TEST_TAGS:-vmservice}"
    _export TEST_FOCUS "${TEST_FOCUS:-}"

    _log "Common environment variables exported"
}

# ---------------------------------------------------------------------------
# _decrypt_wcp_credentials
#
# Runs decryptK8Pwd.py on vCenter via SSH and stores the output in the
# caller's wcp_raw variable (dynamic scoping — must be declared local by the
# caller).  Returns 1 if the SSH fails or produces empty output so that
# _retry_with_backoff can treat it as a retriable failure.
# ---------------------------------------------------------------------------
_decrypt_wcp_credentials() {
    local _out
    _out=$(sshpass -p "${vc_root_password}" \
            ssh "${_SSH_OPTS[@]}" -o ConnectTimeout=30 -o LogLevel=ERROR \
            "${vc_root_username}@${vc_url}" \
            "/usr/lib/vmware-wcp/decryptK8Pwd.py") || return 1
    [[ -n "${_out}" ]] || return 1
    wcp_raw="${_out}"
}

# ---------------------------------------------------------------------------
# _fetch_supervisor_access <enable_e2e>
#
# SSHes to vCenter to run decryptK8Pwd.py, downloads the supervisor kubeconfig,
# and patches it to use the real supervisor IP (instead of 127.0.0.1).
# Populates: wcp_ip, wcp_password (in _main's scope via dynamic scoping).
# Exports:   KUBECONFIG
# ---------------------------------------------------------------------------
_fetch_supervisor_access() {
    local -r enable_e2e="$1"

    _log "Retrieving WCP/Supervisor cluster credentials..."

    if ! command -v sshpass >/dev/null 2>&1; then
        _log "sshpass not found — attempting to install..."
        if command -v apt-get >/dev/null 2>&1; then
            { apt-get update -qq && apt-get install -y -qq sshpass; } >&2
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q sshpass >&2
        else
            _warn "sshpass unavailable and no recognised package manager found; WCP credential extraction may fail"
        fi
    fi

    local wcp_raw=""
    if ! _retry_with_backoff "${_DECRYPT_RETRY_ATTEMPTS}" "${_DECRYPT_RETRY_INITIAL_DELAY}" \
            "SSH to vCenter (decryptK8Pwd.py)" \
            _decrypt_wcp_credentials; then
        _err "Failed to retrieve WCP credentials from vCenter after ${_DECRYPT_RETRY_ATTEMPTS} attempts"
        return 1
    fi

    # Write into _main's local variables via dynamic scoping.
    wcp_password=$(grep "PWD:" <<< "${wcp_raw}" | sed -E 's/.*PWD: ([^ ]*).*/\1/')
    wcp_ip=$(grep "IP:"  <<< "${wcp_raw}" | sed -E 's/.*IP: ([^ ]*).*/\1/')

    if [[ -z "${wcp_ip}" || -z "${wcp_password}" ]]; then
        _err "Failed to parse Supervisor IP or password from vCenter output"
        return 1
    fi

    _log "Supervisor IP: ${wcp_ip}"

    mkdir -p "${HOME}/.kube"
    local -r kubeconfig_dest="${HOME}/.kube/${vc_url}.kubeconfig"
    local -r wcp_kubeconfig_dest="${HOME}/.kube/wcp-config"

    _log "Downloading kubeconfig to ${kubeconfig_dest}..."
    if ! _retry_with_backoff "${_KUBECONFIG_RETRY_ATTEMPTS}" "${_KUBECONFIG_RETRY_INITIAL_DELAY}" "SCP kubeconfig from supervisor" \
            sshpass -p "${wcp_password}" \
            scp "${_SSH_OPTS[@]}" -o ConnectTimeout=30 \
            "root@${wcp_ip}:~/.kube/config" "${kubeconfig_dest}"; then
        _err "Failed to copy kubeconfig from supervisor cluster after retries"
        return 1
    fi

    # Replace 127.0.0.1 so the kubeconfig is usable from outside the cluster VM.
    case "${OSTYPE:-$(uname -s)}" in
        darwin*|Darwin) sed -i ""  "s/127.0.0.1/${wcp_ip}/g" "${kubeconfig_dest}" ;;
        *)              sed -i     "s/127.0.0.1/${wcp_ip}/g" "${kubeconfig_dest}" ;;
    esac

    _export KUBECONFIG "${kubeconfig_dest}"

    if [[ "${enable_e2e}" == "true" ]]; then
        _log "Copying kubeconfig → ${wcp_kubeconfig_dest} (--e2e)"
        cp "${kubeconfig_dest}" "${wcp_kubeconfig_dest}"
    else
        _log "Skipping kubeconfig copy to ${wcp_kubeconfig_dest} (pass --e2e to enable)"
    fi
}

# ---------------------------------------------------------------------------
# _setup_kubectl_vsphere
#
# Downloads the supervisor-version-specific kubectl-vsphere plugin and
# prepends its bin directory to PATH.
# Reads: wcp_ip (from _main's scope via dynamic scoping).
# ---------------------------------------------------------------------------
_setup_kubectl_vsphere() {
    if [[ -z "${wcp_ip}" ]]; then
        _err "Cannot install kubectl-vsphere: WCP_IP not set"
        return 1
    fi

    _log "Installing kubectl-vsphere from supervisor ${wcp_ip}..."
    local plugin_os
    case "$(uname -s)-$(uname -m)" in
        Darwin-arm64)  plugin_os="darwin-arm64" ;;
        Darwin-x86_64) plugin_os="darwin-amd64" ;;
        *)             plugin_os="linux-amd64"  ;;
    esac

    local -r plugin_url="https://${wcp_ip}/wcp/plugin/${plugin_os}/vsphere-plugin.zip"
    local -r extract_dir="/tmp/vsphere-plugin-$$"

    local curl_err
    if ! curl_err=$(curl --insecure --max-time 60 --retry 3 --retry-delay 5 -fsSLo vsphere-plugin.zip "${plugin_url}" 2>&1); then
        rm -f vsphere-plugin.zip
        _err "Failed to download kubectl-vsphere from ${wcp_ip} (${plugin_os}): ${curl_err}"
        return 1
    fi

    mkdir -p "${extract_dir}"
    local unzip_out
    if ! unzip_out=$(unzip -o vsphere-plugin.zip -d "${extract_dir}" 2>&1); then
        local zip_size
        zip_size=$(wc -c < vsphere-plugin.zip 2>/dev/null || echo "unknown")
        rm -f vsphere-plugin.zip
        _err "Failed to unzip kubectl-vsphere plugin (zip size: ${zip_size} bytes): ${unzip_out}"
        return 1
    fi

    rm -f vsphere-plugin.zip
    export PATH="${extract_dir}/bin:${PATH}"
    # When not sourced, emit PATH so the caller's shell gets it too.
    # Use a literal $PATH so the user's current PATH is spliced in at eval
    # time rather than the subprocess's expanded copy.
    if [[ "${_SOURCED}" == "false" ]]; then
        printf 'export PATH=%q:$PATH\n' "${extract_dir}/bin"
    fi
    _log "kubectl-vsphere available at ${extract_dir}/bin"
}

# ---------------------------------------------------------------------------
# _probe_gateway_ssh <host>
#
# Tries vc_root_old_password then vc_root_password against the gateway VM.
# Writes _gw_password into the caller's scope (via dynamic scoping) on success,
# or returns 1 if neither password works.
# ---------------------------------------------------------------------------
_probe_gateway_ssh() {
    local -r host="$1"
    _gw_password=""

    local pw
    for pw in "${vc_root_old_password:-}" "${vc_root_password}"; do
        [[ -z "${pw}" ]] && continue
        if sshpass -p "${pw}" ssh -T "${_SSH_OPTS[@]}" -o ConnectTimeout=5 \
                "root@${host}" "echo ok" >/dev/null 2>&1; then
            _gw_password="${pw}"
            _log "Gateway SSH authenticated"
            return 0
        fi
    done
    _warn "Could not authenticate to gateway VM — skipping proxy and pykmip install"
    return 1
}

# ---------------------------------------------------------------------------
# _setup_kms_providers <script_dir> <govc_url> <cidr> <gw_password> <mode>
#
# Configures KMS key providers on vCenter.
# mode "full"        → pykmip install + gce2e-standard (KMIP) + gce2e-native
# mode "native-only" → gce2e-native only (no pykmip / gateway VM required)
# ---------------------------------------------------------------------------
_setup_kms_providers() {
    local -r script_dir="$1" govc_url="$2" cidr="$3" gw_password="$4" mode="$5"

    _log "Setting up KMS key providers (${mode})..."
    if [[ "${mode}" == "full" ]]; then
        GOVC_USERNAME="${vc_vim_username}" GOVC_PASSWORD="${vc_vim_password}" \
            GATEWAY_VM_PASSWORD="${gw_password}" \
            "${script_dir}/kms.sh" install "${govc_url}" "${cidr}" >&2 \
            && _log "✓ KMS: gce2e-standard (KMIP) + gce2e-native configured" \
            || _warn "KMS install failed (may already be configured)"
    else
        GOVC_USERNAME="${vc_vim_username}" GOVC_PASSWORD="${vc_vim_password}" \
            "${script_dir}/kms.sh" setup "${govc_url}" "${cidr}" >&2 \
            && _log "✓ KMS: gce2e-native configured" \
            || _warn "KMS setup failed"
    fi
}

# ---------------------------------------------------------------------------
# _setup_gateway_and_proxy <script_dir>
#
# Uses govc to discover the external-gateway VM, probes its SSH password,
# applies a DNS fix for the Broadcom package mirror, installs the squid
# proxy, and exports HTTP_PROXY / related variables.
#
# Always finishes by calling _setup_kms_providers (full or native-only).
# Reads: vc_url, vc_vim_*, vc_root_*, wcp_ip (from _main via dynamic scoping).
# ---------------------------------------------------------------------------
_setup_gateway_and_proxy() {
    local -r script_dir="$1"
    local -r mgmt_cidr='10.0.0.0/8'

    if ! command -v govc >/dev/null 2>&1; then
        _warn "Skipping gateway/proxy and KMS setup: govc not found in PATH"
        return 0
    fi

    local -r govc_url="${vc_vim_username}:${vc_vim_password}@${vc_url}"
    _export GOVC_URL      "${govc_url}"
    _export GOVC_INSECURE "true"

    _log "Discovering gateway VM via govc..."
    local gateway_ip
    gateway_ip=$(GOVC_USERNAME="${vc_vim_username}" GOVC_PASSWORD="${vc_vim_password}" \
        "${script_dir}/proxy.sh" gateway "${vc_url}" "${mgmt_cidr}" 2>&1 \
        | grep "^10\." | head -1 || true)

    if [[ -z "${gateway_ip:-}" || "${gateway_ip}" == "null" ]]; then
        _warn "Could not find gateway VM (may not be a VDS testbed)"
        _setup_kms_providers "${script_dir}" "${govc_url}" "${mgmt_cidr}" "" "native-only"
        return 0
    fi

    _log "Gateway VM IP: ${gateway_ip}"

    local _gw_password=""
    if ! _probe_gateway_ssh "${gateway_ip}"; then
        _setup_kms_providers "${script_dir}" "${govc_url}" "${mgmt_cidr}" "" "native-only"
        return 0
    fi

    # Fix DNS for packages.vcfd.broadcom.net on the gateway VM.
    # The gateway's systemd-resolved has a known TCP-mode bug that breaks pip's
    # access to the internal Broadcom package mirror when resolving this host.
    local -r pkg_host="packages.vcfd.broadcom.net"
    local pkg_ip
    pkg_ip=$(sshpass -p "${_gw_password}" ssh -T "${_SSH_OPTS[@]}" \
        "root@${gateway_ip}" \
        "getent hosts ${pkg_host} 2>/dev/null | awk '{print \$1}' | head -1 || \
         python3 -c \"import socket; print(socket.gethostbyname('${pkg_host}'))\" 2>/dev/null || \
         true" 2>/dev/null || true)
    if [[ -n "${pkg_ip}" ]]; then
        local hosts_entry="${pkg_ip} ${pkg_host}"
        hosts_entry+=" vsphere-docker-virtual.${pkg_host}"
        hosts_entry+=" wcp-gc-docker-local.${pkg_host}"
        hosts_entry+=" vsphere-docker-dev-local.${pkg_host}"
        hosts_entry+=" vcf-kubernetes-service-dev-docker-local.${pkg_host}"
        sshpass -p "${_gw_password}" ssh -T "${_SSH_OPTS[@]}" "root@${gateway_ip}" \
            "sed -i '/${pkg_host}/d' /etc/hosts && echo '${hosts_entry}' >> /etc/hosts" \
            && _log "✓ DNS fix applied on gateway VM (${pkg_host} → ${pkg_ip})" \
            || _warn "Could not apply DNS fix on gateway VM"
    else
        _warn "Could not resolve ${pkg_host} on gateway VM — skipping DNS fix"
    fi

    _log "Installing squid proxy on gateway VM..."
    GOVC_USERNAME="${vc_vim_username}" GOVC_PASSWORD="${vc_vim_password}" \
        GATEWAY_VM_PASSWORD="${_gw_password}" \
        "${script_dir}/proxy.sh" install "${vc_url}" "${mgmt_cidr}" >&2

    # Export proxy variables.  Tests that use HTTP_PROXY only need the gateway
    # IP for direct SSH (port 22) — they do not connect through squid (3128).
    # The runner's subnet is firewalled from 3128 even when squid is healthy,
    # so gating on nc to 3128 would give false negatives.
    local no_proxy_val="localhost,127.0.0.1,${vc_url},${SUPERVISOR_CLUSTER_IP:-}"
    _export HTTP_PROXY          "${gateway_ip}:3128"
    _export HTTPS_PROXY         "${gateway_ip}:3128"
    _export NO_PROXY            "${no_proxy_val}"
    _export no_proxy            "${no_proxy_val}"
    _export GATEWAY_IP          "${gateway_ip}"
    _export GATEWAY_VM_USERNAME "${GATEWAY_VM_USERNAME:-root}"
    _export GATEWAY_VM_PASSWORD "${_gw_password}"
    _log "✓ HTTP_PROXY=${gateway_ip}:3128  NO_PROXY=${no_proxy_val}"

    _setup_kms_providers "${script_dir}" "${govc_url}" "${mgmt_cidr}" "${_gw_password}" "full"
}

# ---------------------------------------------------------------------------
# _setup_e2e <script_dir>
#
# Orchestrates the --e2e steps: kubectl-vsphere install, gateway/proxy setup,
# and KMS key provider configuration.
# ---------------------------------------------------------------------------
_setup_e2e() {
    local -r script_dir="$1"
    _log "=== E2E Testbed Setup ==="
    if ! _retry_with_backoff "${_KUBECTL_VSPHERE_RETRY_ATTEMPTS}" "${_KUBECTL_VSPHERE_RETRY_INITIAL_DELAY}" \
            "Install kubectl-vsphere" _setup_kubectl_vsphere; then
        _err "Failed to install kubectl-vsphere after ${_KUBECTL_VSPHERE_RETRY_ATTEMPTS} attempts"
        return 1
    fi
    _setup_gateway_and_proxy "${script_dir}"
    _log "=== E2E Testbed Setup Complete ==="
}

# ---------------------------------------------------------------------------
# _main
#
# Parses arguments, runs the setup pipeline, and prints a summary.
# All "pipeline state" variables (testbed_data, vc_*, wcp_*, etc.) are
# declared local here so sub-functions can share them via dynamic scoping
# without leaking them into the caller's environment.
# ---------------------------------------------------------------------------
_main() {
    local testbed_source="" enable_e2e=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --e2e)    enable_e2e=true; shift ;;
            --help|-h)
                cat >&2 <<'EOF'
Usage: setup-testbed-env.sh TESTBED_SOURCE [OPTIONS]

Arguments:
  TESTBED_SOURCE  Path to a local testbedinfo.json file or an HTTP(S) URL

Options:
  --e2e       Copy kubeconfig to ~/.kube/wcp-config, install kubectl-vsphere,
              set up the squid HTTP proxy, and configure KMS key providers
  -h, --help  Show this help and exit

Examples:
  # Print export lines to stdout (copy/paste or eval):
  ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json
  eval "$(./hack/e2e/setup-testbed-env.sh ./testbedinfo.json)"

  # Source into the current shell:
  source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json
  source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json --e2e
EOF
                return 0
                ;;
            -*) _err "Unknown option: $1 (use --help)"; return 1 ;;
            *)  testbed_source="$1"; shift ;;
        esac
    done

    _log "=== VM Operator E2E Testbed Environment Setup ==="

    if [[ -z "${testbed_source}" ]]; then
        _err "Testbed source is required."
        _err "Usage: setup-testbed-env.sh TESTBED_SOURCE [--e2e]"
        return 1
    fi

    # Pipeline state: shared with sub-functions via dynamic scoping.
    local testbed_data="" testbed_info=""
    local vc_url="" vc_root_password="" vc_root_old_password=""
    local vc_root_username="" vc_vim_username="" vc_vim_password=""
    local wcp_ip="" wcp_password=""

    _load_testbed "${testbed_source}"  || return
    _parse_vc_credentials              || return
    _export_common_vars
    _fetch_supervisor_access "${enable_e2e}" || return

    # Export WCP/supervisor under both naming conventions used in the test suite.
    _export WCP_IP                  "${wcp_ip}"
    _export WCP_PASSWORD            "${wcp_password}"
    _export SUPERVISOR_CLUSTER_IP   "${wcp_ip}"
    _export SUPERVISOR_CLUSTER_PASSWORD "${wcp_password}"

    if [[ "${enable_e2e}" == "true" ]]; then
        # Resolve the directory containing this script so we can find siblings
        # (proxy.sh, kms.sh).  Cascade: bash BASH_SOURCE[0] → zsh $0 → cwd search.
        local script_dir=""
        if [[ -n "${BASH_VERSION:-}" ]]; then
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        elif [[ -n "${ZSH_VERSION:-}" ]]; then
            script_dir="$(cd "$(dirname "$0")" && pwd)"
        fi
        if [[ -z "${script_dir:-}" || ! -f "${script_dir}/proxy.sh" ]]; then
            local _d
            for _d in "$(pwd)/hack/e2e" "$(pwd)"; do
                if [[ -f "${_d}/proxy.sh" ]]; then
                    script_dir="${_d}"
                    break
                fi
            done
        fi
        _setup_e2e "${script_dir}"
    fi

    # ------------------------------------------------------------------
    # Summary
    # ------------------------------------------------------------------
    _log ""
    _log "=== Setup Complete ==="
    _log "Source:    ${testbed_source}"
    if [[ "${enable_e2e}" == "true" ]]; then
        _log "E2E setup: enabled"
    else
        _log "E2E setup: disabled (pass --e2e to enable)"
    fi

    if [[ "${_SOURCED}" == "false" ]]; then
        _log ""
        _log "The export lines above have been printed to stdout."
        _log "Apply with:  eval \"\$(${0} ${testbed_source}${enable_e2e:+ --e2e})\""
    else
        _log ""
        _log "  VC_URL / VCSA_IP:           ${VC_URL}"
        _log "  VC_ROOT_USERNAME:           ${VC_ROOT_USERNAME}"
        _log "  VC_VIM_USERNAME (govc):     ${VC_VIM_USERNAME}"
        _log "  NETWORK:                    ${NETWORK}"
        _log "  KUBECONFIG:                 ${KUBECONFIG:-not set}"
        _log "  SUPERVISOR_CLUSTER_IP:      ${SUPERVISOR_CLUSTER_IP}"
        _log "  HTTP_PROXY:                 ${HTTP_PROXY:-not set}"
        _log "  GATEWAY_IP:                 ${GATEWAY_IP:-not set}"
    fi
    _log ""
}

# ---------------------------------------------------------------------------
# Entry point.
# Call _main and propagate its exit status correctly whether this script is
# being sourced or executed directly.
# ---------------------------------------------------------------------------
if [[ "${_SOURCED}" == "true" ]]; then
    _main "$@" || return $?
else
    _main "$@"
fi
