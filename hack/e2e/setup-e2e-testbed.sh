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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MGMT_CIDR='10.0.0.0/8'

echo "=== E2E Testbed Setup ==="

# ---------------------------------------------------------------------------
# kubectl-vsphere
# The plugin is supervisor-version-specific so it cannot be baked into the
# image; it must be downloaded at runtime from the Supervisor cluster.
# ---------------------------------------------------------------------------
if [ -n "${WCP_IP:-}" ]; then
    echo "Installing kubectl-vsphere plugin from Supervisor cluster ${WCP_IP}..."
    if curl --max-time 60 --retry 3 --retry-delay 5 -LOk \
            "https://${WCP_IP}/wcp/plugin/linux-amd64/vsphere-plugin.zip" 2>/dev/null; then
        unzip -o vsphere-plugin.zip -d /tmp/vsphere-plugin
        # Try /usr/local/bin (directly or via sudo), fall back to ~/.local/bin
        if mv -f /tmp/vsphere-plugin/bin/* /usr/local/bin/ 2>/dev/null || \
           sudo mv -f /tmp/vsphere-plugin/bin/* /usr/local/bin/ 2>/dev/null; then
            echo "✓ kubectl-vsphere installed to /usr/local/bin"
        else
            echo "⚠ Could not write to /usr/local/bin, installing to ~/.local/bin"
            mkdir -p "${HOME}/.local/bin"
            mv -f /tmp/vsphere-plugin/bin/* "${HOME}/.local/bin/"
            export PATH="${HOME}/.local/bin:${PATH}"
            echo "✓ kubectl-vsphere installed to ${HOME}/.local/bin"
        fi
        rm -rf vsphere-plugin.zip /tmp/vsphere-plugin
    else
        echo "⚠ Failed to download kubectl-vsphere plugin from ${WCP_IP}"
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
    export GOVC_URL="${VC_ROOT_USERNAME}:${VC_ROOT_PASSWORD}@${VC_URL}"
    export GOVC_INSECURE=true

    echo "Discovering gateway VM IP via govc..."
    GATEWAY_IP=$(GOVC_USERNAME="${VC_ROOT_USERNAME}" GOVC_PASSWORD="${VC_ROOT_PASSWORD}" \
        "${SCRIPT_DIR}/proxy.sh" gateway "${VC_URL}" "${MGMT_CIDR}" 2>/dev/null || true)

    if [ -n "${GATEWAY_IP:-}" ] && [ "${GATEWAY_IP}" != "null" ]; then
        echo "Gateway VM IP: ${GATEWAY_IP}"

        # Install squid proxy on gateway and export HTTP_PROXY.
        # Tests using VerifyLoginAndRunCmdsInVDSSetup require HTTP_PROXY to be set.
        echo "Installing squid proxy on gateway VM..."
        GOVC_USERNAME="${VC_ROOT_USERNAME}" GOVC_PASSWORD="${VC_ROOT_PASSWORD}" \
            "${SCRIPT_DIR}/proxy.sh" install "${VC_URL}" "${MGMT_CIDR}" 2>/dev/null \
            || echo "⚠ Proxy install failed (may already be set up)"
        export HTTP_PROXY="${GATEWAY_IP}:3128"
        export HTTPS_PROXY="${GATEWAY_IP}:3128"
        export GATEWAY_IP="${GATEWAY_IP}"
        echo "✓ HTTP_PROXY set to ${HTTP_PROXY}"

        # Full KMS install: deploys pykmip on the gateway VM then registers both
        # gce2e-standard (KMIP) and gce2e-native key providers with vCenter.
        echo "Setting up KMS key providers (with pykmip on gateway VM)..."
        GOVC_USERNAME="${VC_ROOT_USERNAME}" GOVC_PASSWORD="${VC_ROOT_PASSWORD}" \
            "${SCRIPT_DIR}/kms.sh" install "${GOVC_URL}" "${MGMT_CIDR}" 2>/dev/null \
            && echo "✓ KMS key providers configured" \
            || echo "⚠ KMS install failed (may already be configured)"
    else
        echo "⚠ Could not find gateway VM (may not be a VDS testbed)"

        # Without a gateway VM, we can still configure the native key provider
        # directly on vCenter (no pykmip / gce2e-standard).
        echo "Setting up KMS native key provider only..."
        GOVC_USERNAME="${VC_ROOT_USERNAME}" GOVC_PASSWORD="${VC_ROOT_PASSWORD}" \
            "${SCRIPT_DIR}/kms.sh" setup "${GOVC_URL}" "${MGMT_CIDR}" 2>/dev/null \
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
