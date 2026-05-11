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
    pip3 install --upgrade pip
  fi

  pip3 install pykmip

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
