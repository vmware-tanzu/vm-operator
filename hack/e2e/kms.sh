#!/bin/bash

# This script installs and configures a Native and Standard (PyKMIP) Key Provider for use by vCenter.
# To run directly (default vds password below, nsx password is generated):
# % export GOVC_URL="Administrator@vsphere.local:${vc_pass}@${vc_host}"
# % GATEWAY_VM_PASSWORD=vmware ./hack/kms.sh install

set -o errexit
set -o nounset
set -o pipefail
set -x

export GOVC_URL # set in main()
export GOVC_INSECURE=true
GATEWAY_VM_USERNAME="${GATEWAY_VM_USERNAME:-root}"
GATEWAY_VM_PASSWORD="${GATEWAY_VM_PASSWORD:-vmware}"
script_dir="$(dirname "$0")"
crt_dir="$script_dir/tools/bin"

find_gateway_ip() {
  mgmtCidr="$1"

  # VDS:
  #  vm == external-gateway
  # NSX:
  #  vm == external-vm-gateway
  vm=$(govc find / -type m -name external*gateway)

  # Use grepcidr if available, otherwise fallback to grep for common management networks
  if command -v grepcidr >/dev/null 2>&1; then
    govc vm.ip -a -v4 "$vm" | tr ',' '\n' | grepcidr "$mgmtCidr"
  else
    # Fallback: get first non-169.254.x.x IP (avoid link-local)
    govc vm.ip -a -v4 "$vm" | tr ',' '\n' | grep -v "^169\.254\." | head -n1
  fi
}

install() {
  if [ ! -e "$crt_dir/pykmip-crt.pem" ] ; then
    mkdir -p "$crt_dir"
    openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
            -subj "/C=US/ST=CA/L=PA/O=Broadcom/OU=VCF/CN=pykmip" \
            -keyout "$crt_dir"/pykmip-key.pem -out "$crt_dir"/pykmip-crt.pem
  fi

  target="$1@$2"
  password=$3

  sshpass -p "$password" scp -o PubkeyAuthentication=no -o StrictHostKeyChecking=no "$crt_dir"/pykmip-*.pem "$script_dir"/install-pykmip.sh "$target":
  sshpass -p "$password" ssh -o PubkeyAuthentication=no -o StrictHostKeyChecking=no "$target" /bin/bash ./install-pykmip.sh

  setup "$2" || echo "KMS setup failed"
}

# See also: vCenter -> Configure -> Security -> Key Providers
setup() {
  ip="$1"

  name=gce2e-standard
  if ! govc kms.ls "$name" 2> /dev/null ; then
    govc kms.add -n pykmip -a "$ip" "$name"
  fi
  crt=$(cat "$crt_dir/pykmip-crt.pem")
  key=$(cat "$crt_dir/pykmip-key.pem")

  # Note: using the same key pair for the server (pykmip) and client (vCenter)
  govc kms.trust -server-cert "$crt" -client-cert "$crt" -client-key "$key" "$name"
  govc kms.ls "$name"

  name=gce2e-native
  if ! govc kms.ls "$name" 2> /dev/null ; then
    govc kms.add -tpm=false -N "$name"
  fi
  # Take a backup (and throw it away), required to activate the provider
  govc kms.export -f /dev/null "$name"

  govc kms.ls "$name"
}

main() {
  if [ "$#" -ge 2 ]; then
    GOVC_URL="$2"
  fi
  mgmtCidr='10.0.0.0/8'
  if [ "$#" -ge 3 ]; then
    mgmtCidr="$3"
  fi

  case $1 in
    "install")
      install "$GATEWAY_VM_USERNAME" "$(find_gateway_ip "$mgmtCidr")" "$GATEWAY_VM_PASSWORD"
      ;;
    "setup")
      setup "$(find_gateway_ip "$mgmtCidr")"
      ;;
    *)
      echo "unknown command: $1"
      exit 1
      ;;
  esac
}

main "$@"
