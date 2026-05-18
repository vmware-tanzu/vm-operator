#!/bin/bash

# This script installs and configures a Native and Standard (PyKMIP) Key Provider for use by vCenter.
# To run directly (default vds password below, nsx password is generated):
# % export GOVC_URL="Administrator@vsphere.local:${vc_pass}@${vc_host}"
# % GATEWAY_VM_PASSWORD=vmware ./hack/kms.sh install

set -o errexit
set -o nounset
set -o pipefail

export GOVC_URL # set in main()
export GOVC_INSECURE=true
GATEWAY_VM_USERNAME="${GATEWAY_VM_USERNAME:-root}"
# GATEWAY_VM_PASSWORD must be set by the caller (setup-e2e-testbed.sh passes
# the discovered password). No default — empty fails fast.
GATEWAY_VM_PASSWORD="${GATEWAY_VM_PASSWORD:-}"
script_dir="$(dirname "$0")"

# Common SSH/SCP options for all connections to the gateway VM.
# -T: no PTY (avoids "Too many authentication failures" from the SSH agent)
# PubkeyAuthentication=no: force password auth, don't offer agent keys
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password"
crt_dir="$script_dir/tools/bin"

find_gateway_ip() {
  mgmtCidr="$1"

  # VDS:  vm == external-gateway-vds (or external-gateway)
  # NSX:  vm == external-vm-gateway
  # Use suffix wildcard to match all naming variants.
  vm=$(govc find / -type m -name 'external-gateway*' 2>/dev/null || true)
  if [ -z "$vm" ]; then
    vm=$(govc find / -type m -name 'external-vm-gateway*' 2>/dev/null || true)
  fi
  if [ -z "$vm" ]; then
    return 0
  fi

  # Use grepcidr if available, otherwise fallback to grep for common management networks.
  if command -v grepcidr >/dev/null 2>&1; then
    govc vm.ip -a -v4 "$vm" 2>/dev/null | tr ',' '\n' | grepcidr "$mgmtCidr" || true
  else
    # Fallback: get first non-link-local 10.x IP (management network uses 10.0.0.0/8).
    govc vm.ip -a -v4 "$vm" 2>/dev/null | tr ',' '\n' | grep -v "^169\.254\." | grep "^10\." | head -n1 || true
  fi
}

install() {
  # gce2e-standard requires pykmip running on the gateway VM.
  # Skip if already green (idempotent for parallel runners).
  if kms_is_green "gce2e-standard"; then
    echo "KMS provider gce2e-standard already green, skipping pykmip install"
  else
    if [ ! -e "$crt_dir/pykmip-crt.pem" ] ; then
      mkdir -p "$crt_dir"
      openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
              -subj "/C=US/ST=CA/L=PA/O=Broadcom/OU=VCF/CN=pykmip" \
              -keyout "$crt_dir"/pykmip-key.pem -out "$crt_dir"/pykmip-crt.pem
    fi

    target="$1@$2"
    password=$3

    sshpass -p "$password" scp $SSH_OPTS "$crt_dir"/pykmip-*.pem "$script_dir"/install-pykmip.sh "$target":
    sshpass -p "$password" ssh -T $SSH_OPTS "$target" /bin/bash ./install-pykmip.sh \
      || echo "⚠ pykmip install failed — gce2e-standard KMS will not be available"
  fi

  # setup() configures vCenter key providers; kms_is_green checks inside
  # each block make it safe to call from multiple parallel runners.
  setup "$2"
}

# kms_is_green returns 0 if the named provider already exists and has
# OverallStatus == "green", 1 otherwise.  Safe to call from multiple parallel
# containers because it is read-only.
kms_is_green() {
  local name="$1"
  local status
  status=$(govc kms.ls -json "$name" 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('OverallStatus',''))" \
    2>/dev/null || true)
  [ "${status}" = "green" ]
}

# See also: vCenter -> Configure -> Security -> Key Providers
setup() {
  ip="$1"

  # gce2e-standard requires a running pykmip server on the gateway VM.
  # Only configure it when a gateway IP is available; skip silently otherwise.
  if [ -n "${ip:-}" ]; then
    name=gce2e-standard
    if kms_is_green "$name"; then
      echo "KMS provider ${name} already green, skipping setup"
    else
      if ! govc kms.ls "$name" 2> /dev/null ; then
        govc kms.add -n pykmip -a "$ip" "$name"
      fi
      crt=$(cat "$crt_dir/pykmip-crt.pem")
      key=$(cat "$crt_dir/pykmip-key.pem")

      # Note: using the same key pair for the server (pykmip) and client (vCenter)
      govc kms.trust -server-cert "$crt" -client-cert "$crt" -client-key "$key" "$name"
    fi
    govc kms.ls "$name"
  else
    echo "Skipping gce2e-standard KMS setup: no gateway IP available"
  fi

  # gce2e-native is a vCenter-native key provider that does not need an external
  # server. Configure it unconditionally so encryption tests can run even on
  # testbeds that have no VDS gateway VM (e.g. NSX or minimal testbeds).
  name=gce2e-native
  if kms_is_green "$name"; then
    echo "KMS provider ${name} already green, skipping setup"
  else
    if ! govc kms.ls "$name" 2> /dev/null ; then
      govc kms.add -tpm=false -N "$name"
    fi
    # Take a backup (and throw it away), required to activate the provider.
    govc kms.export -f /dev/null "$name"
  fi
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
