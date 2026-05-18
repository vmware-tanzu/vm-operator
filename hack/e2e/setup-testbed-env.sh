#!/usr/bin/env bash
#
# Setup testbed environment for vm-operator E2E tests
# 
# This script fetches testbed information from local files or URLs and prepares the environment
# for running vm-operator E2E tests by setting up kubeconfig and environment variables.
#
#   # Local file:
#   source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json
#
#   # Remote URL:
#   source ./hack/e2e/setup-testbed-env.sh https://example.com/testbed.json
#
#   # Enable E2E kubeconfig setup by copying kubeconfig to ~/.kube/wcp-config
#   # which is used by the E2E test to access the Supervisor cluster.
#   source ./hack/e2e/setup-testbed-env.sh ./testbedinfo.json --e2e
#

set -u

SCRIPT_SOURCED=true
# Function to handle exits gracefully (return if sourced, exit if executed)
script_exit() {
    local exit_code=${1:-1}
    if [ "$SCRIPT_SOURCED" = "true" ]; then
        return "$exit_code"
    else
        exit "$exit_code"
    fi
}

# Parse command line arguments
TESTBED_SOURCE=""
ENABLE_E2E_KUBECONFIG=""

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --e2e)
                ENABLE_E2E_KUBECONFIG="true"
                shift
                ;;
            --help|-h)
                echo "Usage: $0 TESTBED_SOURCE [OPTIONS]"
                echo ""
                echo "Arguments:"
                echo "  TESTBED_SOURCE  Path to local testbedinfo.json or HTTP(S) URL (required)"
                echo ""
                echo "Options:"
                echo "  --e2e           Enable E2E setup: copy kubeconfig to wcp-config, install"
                echo "                  kubectl-vsphere, set up HTTP proxy and KMS key providers"
                echo "  --help, -h      Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0 ./testbedinfo.json --e2e"
                echo "  $0 https://example.com/testbed.json"
                echo "  source $0 ./testbedinfo.json --e2e"
                echo "  source $0 https://example.com/testbed.json"
                shift
                script_exit 0
                ;;
            -*)
                echo "Unknown option: $1"
                echo "Use --help for usage information"
                script_exit 1
                ;;
            *)
                TESTBED_SOURCE="$1"
                shift
                ;;
        esac
    done
}

# Parse arguments (works for both sourced and executed)
parse_args "$@"

echo "=== VM Operator E2E Testbed Environment Setup ==="

# Check that testbed source is provided
if [ -z "$TESTBED_SOURCE" ]; then
    echo "Error: Testbed source is required"
    echo "Usage: $0 TESTBED_SOURCE [--e2e]"
    echo "Examples:"
    echo "  $0 ./testbedinfo.json --e2e"
    echo "  $0 https://example.com/testbed.json"
    script_exit 1
fi

# Determine data source and fetch testbed data
if [[ "$TESTBED_SOURCE" =~ ^https?:// ]]; then
    echo "Fetching testbed data from URL: ${TESTBED_SOURCE}"
    
    if ! testbed_data=$(curl -fsSL "$TESTBED_SOURCE" 2>/dev/null); then
        echo "Error: Failed to fetch testbed data from ${TESTBED_SOURCE}"
        script_exit 1
    fi
    
    echo "Successfully fetched testbed data from URL"
else
    echo "Using local testbed file: ${TESTBED_SOURCE}"
    
    if [ ! -f "$TESTBED_SOURCE" ]; then
        echo "Error: Local testbed file not found: ${TESTBED_SOURCE}"
        script_exit 1
    fi
    
    if ! testbed_data=$(cat "$TESTBED_SOURCE" 2>/dev/null); then
        echo "Error: Failed to read local testbed file: ${TESTBED_SOURCE}"
        script_exit 1
    fi
    
    echo "Successfully loaded local testbed data"
fi

# Parse the testbed data - handle both UTS format and raw vCenter format
# UTS format has deliverable_blob, raw format is direct JSON
if echo "$testbed_data" | jq -e '.deliverable_blob' >/dev/null 2>&1; then
    echo "Detected UTS deliverable_blob format, extracting data..."
    testbed_info=$(echo "$testbed_data" | jq -r '.deliverable_blob')
else
    echo "Detected direct testbed format"
    testbed_info="$testbed_data"
fi

# Extract vCenter connection details - try multiple JSON path patterns
# Pattern 1: WCP testbed format with nested vc array
if echo "$testbed_info" | jq -e '.vc[0]' >/dev/null 2>&1; then
    echo "Using vc[0] format for vCenter details"
    export VC_URL=$(echo "$testbed_info" | jq -r '.vc[0].ip4 // .vc[0].ip // .vc[0].vcenter_ip')
    export VC_ROOT_PASSWORD=$(echo "$testbed_info" | jq -r '.vc[0].password // .vc[0].vcenter_password')
    export VC_ROOT_OLD_PASSWORD=$(echo "$testbed_info" | jq -r '.vc[0].old_password // ""')
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vc[0].username // .vc[0].vcenter_username // "administrator@vsphere.local"')
    # vimUsername/vimPassword are the vSphere API (govc) credentials, distinct from
    # the SSH root credentials stored in username/password.
    export VC_VIM_USERNAME=$(echo "$testbed_info" | jq -r '.vc[0].vimUsername // .vc[0].username // "administrator@vsphere.local"')
    export VC_VIM_PASSWORD=$(echo "$testbed_info" | jq -r '.vc[0].vimPassword // .vc[0].password')
# Pattern 2: Direct format with vcenter_ prefixed fields
elif echo "$testbed_info" | jq -e '.vcenter_ip' >/dev/null 2>&1; then
    echo "Using direct vcenter_ format for vCenter details"
    export VC_URL=$(echo "$testbed_info" | jq -r '.vcenter_ip')
    export VC_ROOT_PASSWORD=$(echo "$testbed_info" | jq -r '.vcenter_password')
    export VC_ROOT_OLD_PASSWORD=$(echo "$testbed_info" | jq -r '.vcenter_old_password // ""')
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vcenter_username // "administrator@vsphere.local"')
    export VC_VIM_USERNAME=$(echo "$testbed_info" | jq -r '.vcenter_username // "administrator@vsphere.local"')
    export VC_VIM_PASSWORD="${VC_ROOT_PASSWORD}"
# Pattern 3: Simple format (for testing/fallback)
else
    echo "Using simple format for vCenter details"
    export VC_URL=$(echo "$testbed_info" | jq -r '.vc_ip // .vcenter_ip // .testbed_ip')
    export VC_ROOT_PASSWORD=$(echo "$testbed_info" | jq -r '.vc_password // .vcenter_password // .password')
    export VC_ROOT_OLD_PASSWORD=""
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vc_username // .vcenter_username // .username // "administrator@vsphere.local"')
    export VC_VIM_USERNAME="${VC_ROOT_USERNAME}"
    export VC_VIM_PASSWORD="${VC_ROOT_PASSWORD}"
fi

# Validate required variables are set and not null
if [ -z "${VC_URL}" ] || [ "${VC_URL}" = "null" ]; then
    echo "Error: Could not extract VC_URL from testbed data"
    echo "Available fields:" 
    echo "$testbed_info" | jq -r 'keys[]' 2>/dev/null || echo "Invalid JSON format"
    script_exit 1
fi

if [ -z "${VC_ROOT_PASSWORD}" ] || [ "${VC_ROOT_PASSWORD}" = "null" ]; then
    echo "Error: Could not extract VC_ROOT_PASSWORD from testbed data"
    script_exit 1
fi

if [ -z "${VC_ROOT_USERNAME}" ] || [ "${VC_ROOT_USERNAME}" = "null" ]; then
    export VC_ROOT_USERNAME="administrator@vsphere.local"
fi

echo "Configuration loaded successfully:"
echo "  VC_URL: ${VC_URL}"
echo "  VC_ROOT_USERNAME: ${VC_ROOT_USERNAME}"

# Set up common environment variables for vm-operator E2E tests
export VCSA_IP="${VC_URL}"
export SSH_PASSWORD="${VC_ROOT_PASSWORD}"
export SSH_USERNAME="${VC_ROOT_USERNAME}"
# govc uses the vSphere API credentials (administrator@vsphere.local), not the SSH root creds.
export GOVC_USERNAME="${VC_VIM_USERNAME}"
export GOVC_PASSWORD="${VC_VIM_PASSWORD}"
export VCSA_PASSWORD="${SSH_PASSWORD}"

# Detect networking type from testbed data
NETWORKING_TYPE=$(echo "$testbed_info" | jq -r '.networking // "vds"')
export NETWORK="${NETWORKING_TYPE}"

# Set test configuration defaults
export TEST_SKIP=""                 # Tests to skip
export TEST_TAGS="vmservice"        # Test tags to run  
export TEST_FOCUS="${TEST_FOCUS:-}" # Test focus pattern

echo "Standard environment variables set for vm-operator E2E tests"

# Get WCP/Supervisor cluster credentials
echo "Retrieving WCP/Supervisor cluster credentials..."

# Install sshpass if not available (in case the container doesn't have it)
if ! command -v sshpass >/dev/null 2>&1; then
    echo "Installing sshpass..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update -qq && apt-get install -y -qq sshpass
    elif command -v yum >/dev/null 2>&1; then
        yum install -y -q sshpass
    else
        echo "Warning: sshpass not found and package manager not detected"
        echo "WCP credential extraction may fail"
    fi
fi

# Extract WCP credentials from vCenter
if ! wcp_info=$(sshpass -p "${SSH_PASSWORD}" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 "${SSH_USERNAME}@${VCSA_IP}" "/usr/lib/vmware-wcp/decryptK8Pwd.py" 2>/dev/null); then
    echo "Warning: Failed to extract WCP credentials via SSH"
    echo "This may be expected if WCP is not fully enabled yet"
    # Don't exit - allow tests to run without kubeconfig if needed
    WCP_PASSWORD=""
    WCP_IP=""
fi

if [ -n "$wcp_info" ]; then
    WCP_PASSWORD="$(echo "$wcp_info" | grep "PWD:" | sed -E "s/.*PWD: ([^ ]*).*/\1/")"
    WCP_IP="$(echo "$wcp_info" | grep "IP:" | sed -E "s/.*IP: ([^ ]*).*/\1/")"
    
    echo "WCP Supervisor IP: ${WCP_IP}, Password: [REDACTED]"
    
    # Set up kubeconfig for supervisor cluster access
    echo "Setting up kubeconfig for Supervisor cluster..."
    
    # Create .kube directory if it doesn't exist
    mkdir -p "$HOME/.kube"
    wcp_kubeconfig_destination="$HOME/.kube/wcp-config"
    kubeconfig_destination="$HOME/.kube/${VCSA_IP}.kubeconfig"

    echo "Downloading kubeconfig from supervisor cluster to ${kubeconfig_destination}"
 
    # Copy kubeconfig from supervisor cluster
    if sshpass -p "${WCP_PASSWORD}" scp -o StrictHostKeyChecking=no -o ConnectTimeout=30 "root@${WCP_IP}:~/.kube/config" "${kubeconfig_destination}" 2>/dev/null; then
        export KUBECONFIG="${kubeconfig_destination}"
        echo "Kubeconfig exported to: ${kubeconfig_destination}"
        
        # Fix the server URL in kubeconfig (replace 127.0.0.1 with actual IP)
        if command -v sed >/dev/null 2>&1; then
            case "${OSTYPE:-$(uname -s)}" in
                darwin*|Darwin)
                    # macOS sed
                    sed -i "" "s/127.0.0.1/${WCP_IP}/g" "${kubeconfig_destination}"
                    ;;
                *)
                    # Linux sed
                    sed -i "s/127.0.0.1/${WCP_IP}/g" "${kubeconfig_destination}"
                    ;;
            esac
            echo "Kubeconfig server URL updated to use ${WCP_IP}"
        fi

        # Only copy kubeconfig to wcp-config if --e2e flag is enabled
        if [ "$ENABLE_E2E_KUBECONFIG" = "true" ]; then
            echo "Copying kubeconfig to ${wcp_kubeconfig_destination} (--e2e flag enabled)"
            cp "${kubeconfig_destination}" "${wcp_kubeconfig_destination}"
        else
            echo "Skipping kubeconfig copy to ${wcp_kubeconfig_destination} (use --e2e flag to enable)"
        fi

    else
        echo "Warning: Failed to copy kubeconfig from supervisor cluster"
        echo "Tests requiring kubectl access may fail"
    fi
else
    echo "Warning: No WCP credentials found - supervisor cluster may not be ready"
fi

# Export additional variables that vm-operator tests might expect
export SUPERVISOR_CLUSTER_IP="${WCP_IP:-}"
export SUPERVISOR_CLUSTER_PASSWORD="${WCP_PASSWORD:-}"

# Run E2E-specific setup when --e2e is passed:
#   - install kubectl-vsphere plugin
#   - set up squid proxy on gateway VM and export HTTP_PROXY
#   - configure KMS key providers (gce2e-native, gce2e-standard)
if [ "$ENABLE_E2E_KUBECONFIG" = "true" ]; then
    # Resolve the directory containing THIS script, compatible with both bash
    # (BASH_SOURCE[0]) and zsh. We use a cascade:
    #  1. bash: BASH_SOURCE[0] gives the actual script path even when sourced
    #  2. zsh:  $0 gives the script path when sourced with `source`
    #  3. fallback: search for sibling setup-e2e-testbed.sh relative to cwd
    _SCRIPT_DIR=""
    # bash path - evaluated in a subshell to avoid set -u errors in zsh
    if [ -n "${BASH_VERSION:-}" ]; then
        _SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    elif [ -n "${ZSH_VERSION:-}" ]; then
        # In zsh, $0 when sourced is the path of the sourced file
        _SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    fi
    # If still can't find the sibling, search relative to cwd
    if [ -z "${_SCRIPT_DIR}" ] || [ ! -f "${_SCRIPT_DIR}/setup-e2e-testbed.sh" ]; then
        for _dir in "$(pwd)/hack/e2e" "$(pwd)"; do
            if [ -f "${_dir}/setup-e2e-testbed.sh" ]; then
                _SCRIPT_DIR="${_dir}"
                break
            fi
        done
    fi
    # shellcheck source=hack/e2e/setup-e2e-testbed.sh
    export _SCRIPT_DIR
    source "${_SCRIPT_DIR}/setup-e2e-testbed.sh"
fi

echo ""
echo "=== Environment Setup Complete ==="
echo "Data source: ${TESTBED_SOURCE}"
if [ "${ENABLE_E2E_KUBECONFIG}" = "true" ]; then
    echo "E2E setup: Enabled (--e2e)"
else
    echo "E2E setup: Disabled (use --e2e to enable)"
fi
echo ""
echo "The following variables are now available:"
echo "  VC_URL/VCSA_IP: ${VC_URL}"
echo "  VC_ROOT_USERNAME/SSH_USERNAME: ${VC_ROOT_USERNAME}"
echo "  VC_VIM_USERNAME (govc): ${VC_VIM_USERNAME}"
echo "  NETWORK: ${NETWORK}"
echo "  KUBECONFIG: ${KUBECONFIG:-not set}"
echo "  SUPERVISOR_CLUSTER_IP: ${SUPERVISOR_CLUSTER_IP:-not set}"
echo "  HTTP_PROXY: ${HTTP_PROXY:-not set}"
echo "  GATEWAY_IP: ${GATEWAY_IP:-not set}"
echo ""
echo "You can now run vm-operator E2E tests with these credentials."
echo "Example: make e2e-smoke"
echo ""