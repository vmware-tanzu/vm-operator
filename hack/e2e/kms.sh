#!/bin/bash

# This script installs and configures a Native and Standard (PyKMIP) Key Provider for use by vCenter.
# To run directly:
# % export GOVC_URL="Administrator@vsphere.local:${vc_pass}@${vc_host}"
# % GATEWAY_VM_PASSWORD=vmware ./hack/e2e/kms.sh install
#
# The "install" command configures both providers. The "setup" command
# configures only the Native provider (gce2e-native), which needs no PyKMIP
# server or gateway VM.
#
# After the Standard provider (gce2e-standard) is configured, the script waits
# for vCenter to report it green. If a step fails or the provider does not
# become green in time, the provider is torn down (removed from vCenter) and
# the deployment is attempted again, up to KMS_MAX_ATTEMPTS times in total. The
# wait before each new attempt doubles, starting at KMS_RETRY_DELAY seconds.
# Every failure and every retry is logged, and the script exits non-zero when
# the last attempt fails.

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
# Total number of times a KMS deployment is attempted before giving up.
KMS_MAX_ATTEMPTS="${KMS_MAX_ATTEMPTS:-4}"
# Seconds to wait for gce2e-standard to become green after it is configured.
KMS_GREEN_TIMEOUT="${KMS_GREEN_TIMEOUT:-120}"
# Seconds to wait after the first failed attempt before the next one. The wait
# doubles after each further failure, e.g. 30s, 60s and then 120s with 4
# attempts.
KMS_RETRY_DELAY="${KMS_RETRY_DELAY:-30}"
# Seconds to wait for each SSH connection, so that an unreachable host fails an
# attempt in seconds instead of after the kernel's TCP connect timeout.
SSH_CONNECT_TIMEOUT="${SSH_CONNECT_TIMEOUT:-30}"
script_dir="$(dirname "$0")"

# Common SSH/SCP options for all connections to the gateway VM.
# -T: no PTY (avoids "Too many authentication failures" from the SSH agent)
# PubkeyAuthentication=no: force password auth, don't offer agent keys
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password -o ConnectTimeout=${SSH_CONNECT_TIMEOUT}"
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
  local dedicated=false pykmip_down=false
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
    if ! sshpass -p "$password" ssh -T $SSH_OPTS "$target" \
      "PIP_INDEX_URL=${PIP_INDEX_URL:-} /bin/bash ./install-pykmip.sh"; then
      echo "⚠ pykmip install failed — gce2e-standard KMS will not be available"
      # install-pykmip.sh can also fail after it has started the service, for
      # example while adding the iptables rules, so only give up on
      # gce2e-standard if the service is not running either.
      if ! sshpass -p "$password" ssh -T $SSH_OPTS "$target" 'systemctl is-active --quiet pykmip'; then
        pykmip_down=true
      fi
    fi
  fi

  if [ "$pykmip_down" = true ]; then
    # gce2e-standard cannot become green without a running PyKMIP server, so
    # don't register it and wait for it. Configure gce2e-native, which doesn't
    # depend on it, and fail the attempt so that deploy_with_retries tears down
    # and tries again.
    log "✗ PyKMIP is not running on ${target_ip}, not configuring gce2e-standard"
    setup ""
    return 1
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

# log prints a timestamped message so that failures and retries can be located
# in the (usually much longer) testbed setup output.
log() {
  echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) kms.sh: $*"
}

# kms_is_green returns 0 if the named provider already exists and has
# status.overallStatus == "green", 1 otherwise.  Safe to call from multiple
# parallel containers because it is read-only.
#
# "govc kms.ls -json NAME" nests the cluster status under a top-level "status"
# key, i.e. {"info": {...}, "status": {"overallStatus": "green", ...}}.
kms_is_green() {
  local name="$1"
  local status
  status=$(govc kms.ls -json "$name" 2>/dev/null \
    | jq -r '.status.overallStatus // ""' 2>/dev/null || true)
  [ "${status}" = "green" ]
}

# kms_describe prints the overall status of the named provider and, for each of
# its KMIP servers, the status and the connection error reported by vCenter.
kms_describe() {
  local name="$1" out
  out=$(govc kms.ls -json "$name" 2>&1) || {
    echo "  govc kms.ls ${name} failed: ${out}"
    return 0
  }
  jq -r '(.status // {}) as $s
    | ("  overallStatus=\($s.overallStatus // "")"),
      (($s.servers // [])[]
        | "  server \(.name): status=\(.status) connectionStatus=\(.connectionStatus | tojson)")' \
    <<< "$out" || echo "  unexpected govc kms.ls output: ${out}"
}

# kms_wait_green polls until the named provider is green or KMS_GREEN_TIMEOUT
# seconds have passed, in which case it returns 1.
kms_wait_green() {
  local name="$1" deadline=$((SECONDS + KMS_GREEN_TIMEOUT))
  until kms_is_green "$name"; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      return 1
    fi
    sleep 5
  done
}

# teardown removes the gce2e-standard provider from vCenter so that the next
# attempt registers and trusts it from scratch. A provider that is green is
# kept, because another runner sharing this vCenter may have fixed it, or may
# be using it. gce2e-native has no external dependency and setup() is
# idempotent for it, so it is left alone.
teardown() {
  local name=gce2e-standard
  if ! govc kms.ls "$name" > /dev/null 2>&1; then
    log "KMS provider ${name} is not registered, nothing to tear down"
  elif kms_is_green "$name"; then
    log "KMS provider ${name} is green, keeping it"
  else
    log "Tearing down KMS provider ${name}"
    govc kms.rm "$name" || log "⚠ could not remove KMS provider ${name}"
  fi
}

# deploy_with_retries runs the given kms.sh command (install or setup). If it
# fails, which includes the gce2e-standard provider not becoming green, the
# failed attempt is torn down and the command is run again, up to
# KMS_MAX_ATTEMPTS times in total. Returns 1 if the last attempt fails.
deploy_with_retries() {
  local attempt=1 delay="$KMS_RETRY_DELAY" rc

  while true; do
    log "KMS deployment attempt ${attempt}/${KMS_MAX_ATTEMPTS}: $1"

    # errexit is ignored for everything that runs as part of an "if", "&&" or
    # "||" command, even in a subshell, so a failed step would not stop the
    # attempt. Run the attempt as a plain subshell command with errexit enabled
    # inside it, and disable errexit here to read its exit status.
    set +o errexit
    (
      set -o errexit
      "$@"
    )
    rc=$?
    set -o errexit

    if [ "$rc" -eq 0 ]; then
      log "✓ KMS deployment succeeded on attempt ${attempt}/${KMS_MAX_ATTEMPTS}"
      return 0
    fi

    log "⚠ KMS deployment attempt ${attempt}/${KMS_MAX_ATTEMPTS} failed (exit status ${rc})"
    if [ "$attempt" -ge "$KMS_MAX_ATTEMPTS" ]; then
      log "✗ KMS deployment failed after ${attempt} attempt(s), giving up"
      return 1
    fi

    teardown
    log "Retrying KMS deployment in ${delay}s"
    sleep "$delay"
    delay=$((delay * 2))
    attempt=$((attempt + 1))
  done
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

      # Registering and trusting the provider does not prove that vCenter can
      # reach the KMIP server. Fail the attempt, so that deploy_with_retries
      # tears it down and tries again, if it does not become green.
      log "Waiting up to ${KMS_GREEN_TIMEOUT}s for KMS provider ${name} to become green"
      if ! kms_wait_green "$name"; then
        log "✗ KMS provider ${name} is not green after ${KMS_GREEN_TIMEOUT}s:"
        kms_describe "$name"
        return 1
      fi
      log "✓ KMS provider ${name} is green"
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
  # Fail loudly, and before the retry loop, if a tool that the status checks
  # depend on is missing. Their errors are otherwise swallowed, so a missing
  # tool would only show up as a provider that is never reported green.
  local tool
  for tool in govc jq; do
    if ! command -v "$tool" > /dev/null 2>&1; then
      log "✗ required tool not found in PATH: ${tool}"
      return 1
    fi
  done

  if [ "$#" -ge 2 ]; then
    GOVC_URL="$2"
  fi
  mgmtCidr='10.0.0.0/8'
  if [ "$#" -ge 3 ]; then
    mgmtCidr="$3"
  fi

  case $1 in
    "install")
      deploy_with_retries install "$GATEWAY_VM_USERNAME" "$(find_gateway_ip "$mgmtCidr")" "$GATEWAY_VM_PASSWORD"
      ;;
    "setup")
      # Configure gce2e-native only. gce2e-standard needs the PyKMIP
      # certificate and key that install generates or fetches, so a bare setup
      # run, which has none, cannot configure it even when a gateway VM exists.
      deploy_with_retries setup ""
      ;;
    *)
      echo "unknown command: $1"
      exit 1
      ;;
  esac
}

main "$@"
