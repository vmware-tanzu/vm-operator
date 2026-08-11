#!/bin/bash

# This script installs and configures a Native and Standard (PyKMIP) Key Provider for use by vCenter.
# To run directly:
# % export GOVC_URL="Administrator@vsphere.local:${vc_pass}@${vc_host}"
# % GATEWAY_VM_PASSWORD=vmware ./hack/kms.sh install

set -o errexit
set -o nounset
set -o pipefail

export GOVC_URL # set in main()
export GOVC_INSECURE=true
GATEWAY_VM_USERNAME="${GATEWAY_VM_USERNAME:-root}"
# GATEWAY_VM_PASSWORD must be set by the caller (setup-testbed-env.sh passes
# the discovered password). No default — empty fails fast.
GATEWAY_VM_PASSWORD="${GATEWAY_VM_PASSWORD:-}"
# When the testbed already has a dedicated PyKMIP server VM (provisioned by
# the testbed deployer; see setup-testbed-env.sh:_parse_pykmip_server),
# pykmip is installed/configured against it directly instead of the gateway
# VM. No default — empty means "no dedicated VM, use gateway".
PYKMIP_HOST_IP="${PYKMIP_HOST_IP:-}"
PYKMIP_HOST_USERNAME="${PYKMIP_HOST_USERNAME:-root}"
PYKMIP_HOST_PASSWORD="${PYKMIP_HOST_PASSWORD:-}"
script_dir="$(dirname "$0")"

# Common SSH/SCP options for all connections to the gateway VM.
# -T: no PTY (avoids "Too many authentication failures" from the SSH agent)
# PubkeyAuthentication=no: force password auth, don't offer agent keys
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password"
#crt_dir="$script_dir/tools/bin"
crt_dir=$(mktemp -d)

find_gateway_ip() {
  mgmtCidr="$1"

  # VDS:  vm == external-gateway-vds (or external-gateway)
  # NSX:  vm == external-vm-gateway
  # Use suffix wildcard to match all naming variants.
  vm=$(govc find / -type m -name 'external-gateway*' 2>/dev/null | head -n1 || true)
  if [ -z "$vm" ]; then
    vm=$(govc find / -type m -name 'external-vm-gateway*' 2>/dev/null | head -n1 || true)
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
  # Prefer a dedicated PyKMIP server supplied via testbedInfo.json (see
  # setup-testbed-env.sh's _parse_pykmip_server / PYKMIP_HOST_* env vars) over
  # installing pykmip on the external gateway VM discovered via govc. The
  # dedicated server is already provisioned with pykmip running and its own
  # cert, so it skips install-pykmip.sh entirely (see the dedicated branch
  # below); the gateway VM still gets the full install + generated cert.
  local target_user="$1" target_ip="$2" target_password="$3"
  local dedicated=false
  if [ -n "${PYKMIP_HOST_IP:-}" ]; then
    target_user="${PYKMIP_HOST_USERNAME:-root}"
    target_ip="${PYKMIP_HOST_IP}"
    target_password="${PYKMIP_HOST_PASSWORD:-}"
    dedicated=true
    echo "Using dedicated PyKMIP server from testbedInfo.json: ${target_ip}"
  fi

  # gce2e-standard requires pykmip running on the target host.
  # Skip if already green (idempotent for parallel runners).
  if kms_is_green "gce2e-standard"; then
    echo "KMS provider gce2e-standard already green, skipping pykmip install"
  elif [ -z "$target_ip" ]; then
    echo "⚠ No gateway IP available — skipping pykmip install for gce2e-standard"
  elif [ "$dedicated" = true ]; then
    # Don't reinstall pykmip or push a freshly-generated cert that wouldn't
    # match what the dedicated host actually presents over TLS. Just make
    # sure the service is up and pull its existing server/client cert into
    # $crt_dir so setup() trusts the cert the host is actually serving.
    ensure_pykmip_running "$target_user" "$target_ip" "$target_password"
    fetch_pykmip_certs "$target_user" "$target_ip" "$target_password"
  else
    if [ ! -e "$crt_dir/pykmip-crt.pem" ] ; then
      mkdir -p "$crt_dir"
      openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
              -subj "/C=US/ST=CA/L=PA/O=Broadcom/OU=VCF/CN=pykmip" \
              -keyout "$crt_dir"/pykmip-key.pem -out "$crt_dir"/pykmip-crt.pem
    fi

    target="$target_user@$target_ip"
    password="$target_password"

    sshpass -p "$password" scp $SSH_OPTS "$crt_dir"/pykmip-*.pem "$script_dir"/install-pykmip.sh "$target":
    sshpass -p "$password" ssh -T $SSH_OPTS "$target" \
      "PIP_INDEX_URL=${PIP_INDEX_URL:-} /bin/bash ./install-pykmip.sh" \
      || echo "⚠ pykmip install failed — gce2e-standard KMS will not be available"
  fi

  # setup() configures vCenter key providers; kms_is_green checks inside
  # each block make it safe to call from multiple parallel runners.
  setup "$target_ip"
}

# ensure_pykmip_running starts the pykmip systemd service on a dedicated
# PyKMIP host if it isn't already running. It assumes the package, config, and pykmip systemd unit already present
ensure_pykmip_running() {
  local user="$1" ip="$2" password="$3"
  sshpass -p "$password" ssh -T $SSH_OPTS "${user}@${ip}" \
    'systemctl is-active --quiet pykmip || systemctl start pykmip' \
    || echo "⚠ could not confirm/start pykmip service on ${ip}"
}

# fetch_pykmip_certs reads the server certificate/key already configured on
# a dedicated PyKMIP host (paths taken from its server.conf) into $crt_dir,
# so setup()'s govc kms.trust call uses the cert the host actually presents
# instead of one generated locally that would not match.
fetch_pykmip_certs() {
  local user="$1" ip="$2" password="$3"
  mkdir -p "$crt_dir"

  local cert_path key_path
  cert_path=$(sshpass -p "$password" ssh -T $SSH_OPTS "${user}@${ip}" \
    "grep -E '^certificate_path' /etc/pykmip/server.conf | cut -d= -f2")
  key_path=$(sshpass -p "$password" ssh -T $SSH_OPTS "${user}@${ip}" \
    "grep -E '^key_path' /etc/pykmip/server.conf | cut -d= -f2")

  if [ -z "$cert_path" ] || [ -z "$key_path" ]; then
    echo "⚠ could not determine pykmip cert/key paths on ${ip} — gce2e-standard KMS will not be available"
    return 0
  fi

  sshpass -p "$password" ssh -T $SSH_OPTS "${user}@${ip}" "cat '$cert_path'" > "$crt_dir/pykmip-crt.pem"
  sshpass -p "$password" ssh -T $SSH_OPTS "${user}@${ip}" "cat '$key_path'" > "$crt_dir/pykmip-key.pem"
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

  # gce2e-standard requires a running pykmip server — on the gateway VM, or on
  # a dedicated PyKMIP server VM when install() was pointed at one (either
  # way it was just installed with the same self-signed cert_dir cert/key).
  # Only configure it when a target IP is available; skip silently otherwise.
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
    echo "Skipping gce2e-standard KMS setup: no target IP available"
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
