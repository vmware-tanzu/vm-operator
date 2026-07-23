#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -x

# Install https://github.com/OpenKMIP/PyKMIP
# Used as KMS for encrypting VMs

# flock serializes this entire block across parallel SSH sessions.
# The first runner to acquire the lock installs PyKMIP and generates the TLS
# cert; every subsequent runner finds both already present and exits early.
# This prevents two races at once:
#   1. pip uninstall race — parallel runners both try to uninstall/upgrade
#      typing_extensions from system site-packages, leaving a half-removed
#      package that causes an OSError on the slower runner.
#   2. cert race — parallel runners generate different certs; the one whose
#      govc kms.trust runs last wins, but PyKMIP may be serving the other cert.
echo "pykmip-install: waiting for lock (PID $$)..."
flock -x /root/pykmip.lock bash -s -- "${PIP_INDEX_URL:-}" <<'LOCKED'
  PIP_INDEX_URL="$1"

  # --- pip install (serialized) ---
  # VDS/photon:  server == /usr/bin/pykmip-server,  pip3 not installed
  # NSX/ubuntu:  server == /usr/local/bin/pykmip-server, pip3 already installed
  if type -p pykmip-server >/dev/null 2>&1; then
    echo "pykmip-install: pykmip-server already installed, skipping pip install"
  else
    echo "pykmip-install: installing PyKMIP via pip..."
    if ! type -p pip3 >/dev/null 2>&1; then
      # On VDS/Photon OS, ensurepip installs pip as a Python module but does
      # not create a pip3 binary in PATH.
      python3 -m ensurepip --upgrade
    fi
    # PIP_INDEX_URL can be set by the caller to redirect pip to an internal
    # package mirror (e.g. a corporate Artifactory instance).
    python3 -m pip install \
      ${PIP_INDEX_URL:+--index-url "$PIP_INDEX_URL"} \
      pykmip
    # currently by default there are no shared ciphers between
    # vCenter/qClient + pykmip, patch the default TLS1.2 suite for now.
    # See: https://bugzilla-vcf.lvn.broadcom.net/show_bug.cgi?id=3428705
    site=$(python3 -c 'import site; print(site.getsitepackages()[0])')
    sed -i -e "s/'ECDHE-RSA-AES128-SHA256',/'ECDHE-RSA-AES128-SHA',/g" \
        "$site"/kmip/services/auth.py
    echo "pykmip-install: pip install complete"
  fi

  # --- TLS cert generation (serialized) ---
  # Cert lives on the gateway so all parallel runners fetch and register the
  # same cert via govc kms.trust, preventing a trust mismatch between vCenter
  # and the PyKMIP server.
  if [ -e /root/pykmip-crt.pem ]; then
    echo "pykmip-install: TLS cert already exists, skipping cert generation"
  else
    echo "pykmip-install: generating TLS cert..."
    openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
            -subj "/C=US/ST=CA/L=PA/O=Broadcom/OU=VCF/CN=pykmip" \
            -keyout /root/pykmip-key.pem -out /root/pykmip-crt.pem
    echo "pykmip-install: TLS cert generated"
  fi
LOCKED
echo "pykmip-install: lock released (PID $$)"

mkdir -p /etc/pykmip

cat > /etc/pykmip/server.conf <<EOF
[server]
hostname=0.0.0.0
port=5696
ca_path=/root/pykmip-crt.pem
certificate_path=/root/pykmip-crt.pem
key_path=/root/pykmip-key.pem
auth_suite=TLS1.2
policy_path=/etc/pykmip
enable_tls_client_auth=False
logging_level=DEBUG
database_path=/etc/pykmip/pykmip_server.db
EOF

cat > /lib/systemd/system/pykmip.service <<EOF
[Unit]
Description=PyKMIP
After=network-online.target

[Service]
Type=simple
ExecStart=$(type -p pykmip-server | head -n1)

[Install]
WantedBy=multi-user.target
EOF

pushd /etc/systemd/system/multi-user.target.wants >/dev/null
if [ ! -e pykmip.service ] ; then
  ln -s /lib/systemd/system/pykmip.service .
fi
popd >/dev/null

systemctl restart pykmip
systemctl status pykmip

iptables -A INPUT -p tcp --dport 5696 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT
iptables -A OUTPUT -p tcp --sport 5696 -m conntrack --ctstate ESTABLISHED -j ACCEPT
iptables --list
