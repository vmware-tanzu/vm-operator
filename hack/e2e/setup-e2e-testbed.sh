#!/usr/bin/env bash
#
# E2E-specific testbed setup for vm-operator E2E tests.
#
# Installs kubectl-vsphere, sets up the squid HTTP proxy on the gateway VM,
# and configures KMS key providers (gce2e-native, gce2e-standard) on vCenter.
#
# This script must be run AFTER setup-testbed-env.sh has exported:
#   VC_URL, VC_ROOT_USERNAME, VC_ROOT_PASSWORD, WCP_IP
#
# It is called automatically by setup-testbed-env.sh when --e2e is passed.
#
# Usage (direct):
#   source ./hack/e2e/setup-e2e-testbed.sh
#

set -uo pipefail

# Resolve SCRIPT_DIR: works when executed directly (bash or zsh) and when
# sourced from setup-testbed-env.sh. The parent script passes _SCRIPT_DIR.
if [ -n "${_SCRIPT_DIR:-}" ] && [ -f "${_SCRIPT_DIR}/kms.sh" ]; then
    SCRIPT_DIR="${_SCRIPT_DIR}"
elif [ -n "${BASH_VERSION:-}" ] && [ -n "${BASH_SOURCE[0]:-}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
elif [ -n "${ZSH_VERSION:-}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
else
    SCRIPT_DIR="$(pwd)"
fi
MGMT_CIDR='10.0.0.0/8'

echo "=== E2E Testbed Setup ==="

# ---------------------------------------------------------------------------
# kubectl-vsphere
# The plugin is supervisor-version-specific so it cannot be baked into the
# image; it must be downloaded at runtime from the Supervisor cluster.
# ---------------------------------------------------------------------------
if [ -n "${WCP_IP:-}" ]; then
    echo "Installing kubectl-vsphere plugin from Supervisor cluster ${WCP_IP}..."
    # Select the right plugin bundle for the current OS/arch.
    case "$(uname -s)-$(uname -m)" in
        Darwin-arm64)  _plugin_os="darwin-arm64"  ;;
        Darwin-x86_64) _plugin_os="darwin-amd64"  ;;
        *)             _plugin_os="linux-amd64"   ;;
    esac
    _plugin_url="https://${WCP_IP}/wcp/plugin/${_plugin_os}/vsphere-plugin.zip"
    _extract_dir="/tmp/vsphere-plugin-$$"
    if curl --max-time 60 --retry 3 --retry-delay 5 -LOk "${_plugin_url}" 2>/dev/null; then
        mkdir -p "${_extract_dir}"
        unzip -o vsphere-plugin.zip -d "${_extract_dir}" >/dev/null 2>&1
        # Just export PATH to wherever the binaries landed — no moving, no sudo.
        export PATH="${_extract_dir}/bin:${PATH}"
        rm -f vsphere-plugin.zip
        echo "✓ kubectl-vsphere available at ${_extract_dir}/bin"
    else
        echo "⚠ Failed to download kubectl-vsphere plugin from ${WCP_IP} (${_plugin_os})"
    fi
else
    echo "⚠ Skipping kubectl-vsphere install: WCP_IP not set"
fi

# ---------------------------------------------------------------------------
# Gateway VM / HTTP proxy and KMS key providers
# Both require govc to discover the external-gateway VM in the testbed.
# ---------------------------------------------------------------------------
GATEWAY_IP=""
if [ -n "${VC_URL:-}" ] && command -v govc >/dev/null 2>&1; then
    export GOVC_URL="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}:${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}@${VC_URL}"
    export GOVC_INSECURE=true

    echo "Discovering gateway VM IP via govc..."
    GATEWAY_IP=$(GOVC_USERNAME="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}" GOVC_PASSWORD="${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}" \
        "${SCRIPT_DIR}/proxy.sh" gateway "${VC_URL}" "${MGMT_CIDR}" 2>&1 | grep "^10\." | head -1 || true)

    if [ -n "${GATEWAY_IP:-}" ] && [ "${GATEWAY_IP}" != "null" ]; then
        echo "Gateway VM IP: ${GATEWAY_IP}"

        # The gateway VM uses the same Nimbus-managed password as the vCenter
        # host. Try old_password first (Nimbus may not have rotated it yet on
        # the gateway), then fall back to the current password.
        echo "Probing gateway VM SSH password..."
        _gw_password=""
        # Use a function so SSH options are individual args in all shells
        # (avoids zsh's no-word-split behavior for unquoted variables).
        _gw_ssh_probe() {
            sshpass -p "$1" ssh -T \
                -o StrictHostKeyChecking=no \
                -o UserKnownHostsFile=/dev/null \
                -o PubkeyAuthentication=no \
                -o PreferredAuthentications=password \
                -o ConnectTimeout=5 \
                "root@${GATEWAY_IP}" "echo ok" >/dev/null 2>&1
        }
        for _pw in "${VC_ROOT_OLD_PASSWORD:-}" "${VC_ROOT_PASSWORD:-}"; do
            [ -z "${_pw}" ] && continue
            if _gw_ssh_probe "${_pw}"; then
                _gw_password="${_pw}"
                echo "✓ Gateway VM SSH password found"
                break
            fi
        done
        if [ -z "${_gw_password}" ]; then
            echo "⚠ Could not authenticate to gateway VM — skipping proxy and pykmip install"
        else
            # Fix DNS for packages.vcfd.broadcom.net on the gateway VM so that
            # pip can reach the internal Broadcom package mirror. The gateway
            # VM's systemd-resolved has a known bug with TCP-mode DNS responses,
            # which this hostname triggers. We resolve it on the gateway itself
            # (not the runner container) so the fix works regardless of the
            # runner's own DNS configuration.
            echo "Fixing DNS for packages.vcfd.broadcom.net on gateway VM..."
            _pkg_host="packages.vcfd.broadcom.net"
            _pkg_ip=$(sshpass -p "${_gw_password}" ssh -T \
                -o StrictHostKeyChecking=no \
                -o UserKnownHostsFile=/dev/null \
                -o PubkeyAuthentication=no \
                -o PreferredAuthentications=password \
                "root@${GATEWAY_IP}" \
                "getent hosts ${_pkg_host} 2>/dev/null | awk '{print \$1}' | head -1 || \
                 python3 -c \"import socket; print(socket.gethostbyname('${_pkg_host}'))\" 2>/dev/null || true" \
                2>/dev/null || true)
            if [ -n "${_pkg_ip}" ]; then
                _hosts_entry="${_pkg_ip} ${_pkg_host} vsphere-docker-virtual.${_pkg_host} wcp-gc-docker-local.${_pkg_host} vsphere-docker-dev-local.${_pkg_host} vcf-kubernetes-service-dev-docker-local.${_pkg_host}"
                sshpass -p "${_gw_password}" ssh -T \
                    -o StrictHostKeyChecking=no \
                    -o UserKnownHostsFile=/dev/null \
                    -o PubkeyAuthentication=no \
                    -o PreferredAuthentications=password \
                    "root@${GATEWAY_IP}" \
                    "sed -i '/${_pkg_host}/d' /etc/hosts && echo '${_hosts_entry}' >> /etc/hosts" \
                    && echo "✓ DNS fix applied on gateway VM (${_pkg_host} → ${_pkg_ip})" \
                    || echo "⚠ Could not apply DNS fix on gateway VM"
            else
                echo "⚠ Could not resolve ${_pkg_host} on gateway VM — skipping DNS fix"
            fi

            echo "Installing squid proxy on gateway VM..."
            GOVC_USERNAME="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}" GOVC_PASSWORD="${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}" \
                GATEWAY_VM_PASSWORD="${_gw_password}" \
                "${SCRIPT_DIR}/proxy.sh" install "${VC_URL}" "${MGMT_CIDR}"
        fi

        # Export HTTP_PROXY as soon as the gateway IP is confirmed reachable via
        # SSH. Tests that use this env var (VerifyLoginAndRunCmdsInVDSSetup) only
        # need the gateway IP — they SSH directly to port 22, they do NOT connect
        # through the squid forward proxy on port 3128. Gating on nc to port 3128
        # from the runner container is wrong: the runner's subnet is firewalled
        # from 3128 even when squid is healthy on the gateway.
        if [ -n "${_gw_password}" ]; then
            export HTTP_PROXY="${GATEWAY_IP}:3128"
            export HTTPS_PROXY="${GATEWAY_IP}:3128"
            export GATEWAY_IP="${GATEWAY_IP}"
            export GATEWAY_VM_USERNAME="${GATEWAY_VM_USERNAME:-root}"
            export GATEWAY_VM_PASSWORD="${_gw_password}"
            # Build NO_PROXY from the concrete IPs we know about so that kubectl
            # and any other tools can still reach them directly.
            _no_proxy_addrs="localhost,127.0.0.1,${VC_URL:-},${SUPERVISOR_CLUSTER_IP:-}"
            export NO_PROXY="${_no_proxy_addrs}"
            export no_proxy="${_no_proxy_addrs}"
            echo "✓ HTTP_PROXY set to ${HTTP_PROXY}"
            echo "  GATEWAY_VM_USERNAME: ${GATEWAY_VM_USERNAME}"
            echo "  NO_PROXY: ${NO_PROXY}"
        else
            echo "⚠ Gateway VM not reachable via SSH — HTTP_PROXY will not be set"
        fi

        # Full KMS install: deploys pykmip on the gateway VM then registers both
        # gce2e-standard (KMIP) and gce2e-native key providers with vCenter.
        # Only run when we have a working SSH password; fall back to native-only
        # setup otherwise.
        echo "Setting up KMS key providers..."
        if [ -n "${_gw_password}" ]; then
            GOVC_USERNAME="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}" GOVC_PASSWORD="${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}" \
                GATEWAY_VM_PASSWORD="${_gw_password}" \
                "${SCRIPT_DIR}/kms.sh" install "${GOVC_URL}" "${MGMT_CIDR}" \
                && echo "✓ KMS key providers configured" \
                || echo "⚠ KMS install failed (may already be configured)"
        else
            GOVC_USERNAME="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}" GOVC_PASSWORD="${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}" \
                "${SCRIPT_DIR}/kms.sh" setup "${GOVC_URL}" "${MGMT_CIDR}" \
                && echo "✓ KMS native key provider configured" \
                || echo "⚠ KMS setup failed"
        fi
    else
        echo "⚠ Could not find gateway VM (may not be a VDS testbed)"

        # Without a gateway VM, we can still configure the native key provider
        # directly on vCenter (no pykmip / gce2e-standard).
        echo "Setting up KMS native key provider only..."
        GOVC_USERNAME="${VC_VIM_USERNAME:-${VC_ROOT_USERNAME}}" GOVC_PASSWORD="${VC_VIM_PASSWORD:-${VC_ROOT_PASSWORD}}" \
            "${SCRIPT_DIR}/kms.sh" setup "${GOVC_URL}" "${MGMT_CIDR}" \
            && echo "✓ KMS native key provider configured" \
            || echo "⚠ KMS setup failed"
    fi
else
    if ! command -v govc >/dev/null 2>&1; then
        echo "⚠ Skipping gateway/proxy and KMS setup: govc not found in PATH"
    fi
fi

echo "  HTTP_PROXY: ${HTTP_PROXY:-not set}"
echo "  GATEWAY_IP: ${GATEWAY_IP:-not set}"
echo "=== E2E Testbed Setup Complete ==="
