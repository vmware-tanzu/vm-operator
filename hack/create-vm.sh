#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."


################################################################################
##                                     functions
################################################################################


usage() {
    cat <<EOF
Usage: $(basename "${0}") [OPTIONS]

Generates YAML for N VirtualMachine resources using v1alpha5.

OPTIONS:
  -n NUM          Number of VMs to create (default: 1)
  -N NAMESPACE    Namespace for the VMs (default: default)
  -c CLASS        VM class name (default: best-effort-small)
  -i IMAGE        VM image name (default: vmi-0a0044d7c690bcbea)
  -s STORAGE      Storage class (default: wcplocal-storage-profile)
  -h              Show this help message

EXAMPLES:
  # Create a single VM with defaults
  $(basename "${0}")

  # Create 5 VMs in the test namespace
  $(basename "${0}") -n 5 -N test

  # Create 3 VMs with custom class and image
  $(basename "${0}") -n 3 -c my-vm-class -i my-image

EOF
    exit 1
}

create_vms() {
    local group_name="my-vm-group-$(cat /dev/urandom | LC_ALL=C tr -dc 'a-z0-9' | fold -w7 | head -n 1)"
    local count="${1}"
    local namespace="${2}"
    local class_name="${3}"
    local image_name="${4}"
    local storage_class="${5}"
    local -a vm_names=()

    for ((i = 1; i <= count; i++)); do
        if [ "${i}" -gt 1 ]; then
            echo "---"
        fi
        local name="my-vm-$(cat /dev/urandom | LC_ALL=C tr -dc 'a-z0-9' | fold -w7 | head -n 1)"
        vm_names+=("${name}")

        cat <<EOF
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: ${name}
  namespace: ${namespace}
spec:
  className: ${class_name}
  imageName: ${image_name}
  storageClass: ${storage_class}
  groupName: ${group_name}
EOF
    done

    # Emit the VM group
    echo "---"
    cat <<EOF
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineGroup
metadata:
  name: ${group_name}
  namespace: ${namespace}
spec:
  bootOrder:
  - members:
EOF
    for vm_name in "${vm_names[@]}"; do
        echo "    - name: ${vm_name}"
    done
}


################################################################################
##                                      main
################################################################################

# Default values
NUM_VMS=1
NAMESPACE="default"
CLASS_NAME="best-effort-small"
IMAGE_NAME="vmi-0a0044d7c690bcbea"
STORAGE_CLASS="wcplocal-storage-profile"

while getopts ":n:N:c:i:s:h" opt; do
    case $opt in
        "n" ) NUM_VMS="${OPTARG}" ;;
        "N" ) NAMESPACE="${OPTARG}" ;;
        "c" ) CLASS_NAME="${OPTARG}" ;;
        "i" ) IMAGE_NAME="${OPTARG}" ;;
        "s" ) STORAGE_CLASS="${OPTARG}" ;;
        "h" ) usage ;;
        \? )
            echo "Invalid option: -${OPTARG}" >&2
            usage
            ;;
        : )
            echo "Option -${OPTARG} requires an argument." >&2
            usage
            ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]]; then
    echo "Unexpected arguments: $*" >&2
    usage
fi

# Validate NUM_VMS is a positive integer
if ! [[ "${NUM_VMS}" =~ ^[0-9]+$ ]] || [ "${NUM_VMS}" -lt 1 ]; then
    echo "Error: Number of VMs must be a positive integer" >&2
    exit 1
fi

create_vms "${NUM_VMS}" "${NAMESPACE}" "${CLASS_NAME}" "${IMAGE_NAME}" "${STORAGE_CLASS}"

