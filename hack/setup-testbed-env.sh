#!/usr/bin/env bash
#
# Setup testbed environment for vm-operator E2E tests
# 
# This script fetches testbed information from local files or URLs and prepares the environment
# for running vm-operator E2E tests by setting up kubeconfig and environment variables.
#
#   # Local file:
#   source ./hack/setup-testbed-env.sh ./testbedinfo.json
#
#   # Remote URL:
#   source ./hack/setup-testbed-env.sh https://example.com/testbed.json
#
#   # Enable E2E kubeconfig setup by copying kubeconfig to ~/.kube/wcp-config
#   # which is used by the E2E test to access the Supervisor cluster.
#   source ./hack/setup-testbed-env.sh ./testbedinfo.json --e2e
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
                echo "  --e2e           Enable E2E kubeconfig setup (copy kubeconfig to wcp-config)"
                echo "  --help, -h      Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0 ./testbedinfo.json --e2e"
                echo "  $0 https://example.com/testbed.json"
                echo "  source $0 ./testbedinfo.json --e2e"
                echo "  source $0 https://example.com/testbed.json"
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
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vc[0].username // .vc[0].vcenter_username // "administrator@vsphere.local"')
# Pattern 2: Direct format with vcenter_ prefixed fields
elif echo "$testbed_info" | jq -e '.vcenter_ip' >/dev/null 2>&1; then
    echo "Using direct vcenter_ format for vCenter details"
    export VC_URL=$(echo "$testbed_info" | jq -r '.vcenter_ip')
    export VC_ROOT_PASSWORD=$(echo "$testbed_info" | jq -r '.vcenter_password')
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vcenter_username // "administrator@vsphere.local"')
# Pattern 3: Simple format (for testing/fallback)
else
    echo "Using simple format for vCenter details"
    export VC_URL=$(echo "$testbed_info" | jq -r '.vc_ip // .vcenter_ip // .testbed_ip')
    export VC_ROOT_PASSWORD=$(echo "$testbed_info" | jq -r '.vc_password // .vcenter_password // .password')
    export VC_ROOT_USERNAME=$(echo "$testbed_info" | jq -r '.vc_username // .vcenter_username // .username // "administrator@vsphere.local"')
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
export GOVC_PASSWORD="${SSH_PASSWORD}" 
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

echo ""
echo "=== Environment Setup Complete ==="
echo "Data source: ${TESTBED_SOURCE}"
echo "E2E kubeconfig: $(if [ "$ENABLE_E2E_KUBECONFIG" = "true" ]; then echo "Enabled"; else echo "Disabled (use --e2e to enable)"; fi)"
echo ""
echo "The following variables are now available:"
echo "  VC_URL/VCSA_IP: ${VC_URL}"
echo "  VC_ROOT_USERNAME/SSH_USERNAME: ${VC_ROOT_USERNAME}"
echo "  NETWORK: ${NETWORK}"
echo "  KUBECONFIG: ${KUBECONFIG:-not set}"
echo "  SUPERVISOR_CLUSTER_IP: ${SUPERVISOR_CLUSTER_IP:-not set}"
echo ""
echo "You can now run vm-operator E2E tests with these credentials."
echo "Example: make e2e-smoke"
echo ""