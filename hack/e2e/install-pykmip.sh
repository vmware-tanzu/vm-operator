#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -x

# Install https://github.com/OpenKMIP/PyKMIP
# Used as KMS for encrypting VMs

# VDS/photon:
#  server == /usr/bin/pykmip-server
#  pip3 not installed
# NSX/ubuntu:
#  server == /usr/local/bin/pykmip-server
#  pip3 already installed
if ! type -p pykmip-server >/dev/null ; then
  if ! type -p pip3 >/dev/null ; then
    python3 -m ensurepip
    # Best-effort upgrade — may fail if the host has no direct internet access.
    # The version installed by ensurepip is sufficient to install pykmip.
    pip3 install --upgrade \
      ${PIP_INDEX_URL:+--index-url "$PIP_INDEX_URL"} \
      pip || true
  fi

  # PIP_INDEX_URL can be set by the caller to redirect pip to an internal
  # package mirror (e.g. a corporate Artifactory instance). If unset, pip
  # uses its default index (public PyPI).
  #
  # Pin SQLAlchemy to 1.x: pykmip 0.10.0 uses the 1.x Session/Query API
  # which is incompatible with SQLAlchemy 2.0. Without this pin, key
  # creation silently fails with "General Failure" returned to vCenter.
  pip3 install \
    ${PIP_INDEX_URL:+--index-url "$PIP_INDEX_URL"} \
    "pykmip" "sqlalchemy>=1.4,<2.0"

  # currently by default there are no shared ciphers between
  # vCenter/qClient + pykmip, patch the default TLS1.2 suite for now.
  # See: https://bugzilla-vcf.lvn.broadcom.net/show_bug.cgi?id=3428705
  site=$(python3 -c 'import site; print(site.getsitepackages()[0])')
  sed -i -e "s/'ECDHE-RSA-AES128-SHA256',/'ECDHE-RSA-AES128-SHA',/g" \
      "$site"/kmip/services/auth.py
fi

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

pykmip_bin=$(command -v pykmip-server || true)
if [[ -z "$pykmip_bin" ]]; then
  # pip installs to /usr/bin on Photon, /usr/local/bin on Ubuntu
  for p in /usr/bin/pykmip-server /usr/local/bin/pykmip-server; do
    [[ -x "$p" ]] && pykmip_bin="$p" && break
  done
fi
[[ -z "$pykmip_bin" ]] && { echo "pykmip-server not found after install"; exit 1; }

cat > /lib/systemd/system/pykmip.service <<EOF
[Unit]
Description=PyKMIP
After=network-online.target

[Service]
Type=simple
ExecStart=${pykmip_bin}

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

iptables -C INPUT -p tcp --dport 5696 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT 2>/dev/null || \
  iptables -A INPUT -p tcp --dport 5696 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT
iptables -C OUTPUT -p tcp --sport 5696 -m conntrack --ctstate ESTABLISHED -j ACCEPT 2>/dev/null || \
  iptables -A OUTPUT -p tcp --sport 5696 -m conntrack --ctstate ESTABLISHED -j ACCEPT
iptables --list
