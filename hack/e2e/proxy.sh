#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# This script helps you to find out the external-gateway-vm's IP given the
# vCenter IP. It also installs/uninstalls an httpd proxy on the target vm exposed
# at port 3128 with a predefined configuration file.
# Set environmental variable HTTP_PROXY=${GATEWAY_VM_IP}:3128 to use it.
# This files works with an httpd.conf file that lives under the same folder.


SCRIPT_DIR="$(dirname "${BASH_SOURCE}")"
GOVC_INSECURE="${GOVC_INSECURE:-1}"
# GOVC_USERNAME and GOVC_PASSWORD are intentionally NOT defaulted here.
# They must be passed in by the caller (setup-e2e-testbed.sh exports them
# from the testbed JSON before invoking this script). Defaulting to 'vmware'
# would silently override the real testbed credentials and cause govc to fail
# authentication, making find_gateway_vm_path return empty.
GOVC_USERNAME="${GOVC_USERNAME:-}"
GOVC_PASSWORD="${GOVC_PASSWORD:-}"
GATEWAY_VM_USERNAME="${GATEWAY_VM_USERNAME:-root}"
# GATEWAY_VM_PASSWORD must be set by the caller (e.g. setup-e2e-testbed.sh
# passes VC_ROOT_PASSWORD). No default — an empty password fails fast with a
# clear SSH auth error rather than silently using a wrong credential.
GATEWAY_VM_PASSWORD="${GATEWAY_VM_PASSWORD:-}"

# Common SSH/SCP options used for all connections to the gateway VM.
# PubkeyAuthentication=no prevents the SSH agent from offering keys before
# the password, which causes "Too many authentication failures" when the
# agent holds several keys and the server's MaxAuthTries is low.
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password"


find_gateway_vm_path() {
	# This below code is a hack primarily to compensate for the discrepancies that exist in the various testbed deployment
	# methods
	if [[ -z "${GOVC_USERNAME}" || -z "${GOVC_PASSWORD}" ]]; then
		echo "find_gateway_vm_path: GOVC_USERNAME or GOVC_PASSWORD not set; cannot authenticate to vCenter" >&2
		return 1
	fi
	# VDS testbeds name the gateway VM "external-gateway-vds"; NSX uses
	# "external-vm-gateway". Use a suffix wildcard to match all variants.
	vmnames=('external-gateway*' 'external-vm-gateway*')
	for vmname in "${vmnames[@]}"; do
		vmpath=$(GOVC_URL=$vc GOVC_INSECURE=$GOVC_INSECURE GOVC_USERNAME=$GOVC_USERNAME GOVC_PASSWORD=$GOVC_PASSWORD govc find / -type m -name "${vmname}" | head -n1)
		if [[ "${vmpath}" != "" ]]; then
			echo "${vmpath}"
			break
		fi
	done
}

# finds the gateway vm ip in a testbed given a vCenter IP
find_gateway_ip() {
	vc=$1
	mgmtCidr=$2
	GATEWAY_VM_PATH=$(find_gateway_vm_path)
	ips=$(GOVC_URL=$vc GOVC_INSECURE=$GOVC_INSECURE GOVC_USERNAME=$GOVC_USERNAME GOVC_PASSWORD=$GOVC_PASSWORD govc vm.ip -a "${GATEWAY_VM_PATH}")
	# loop over all ips, only return the one that is in the management CIDR
	IFS=',' read -ra ADDR <<< "$ips"
	for i in "${ADDR[@]}"; do
		if command -v grepcidr >/dev/null 2>&1; then
			echo "$i" | grepcidr "$mgmtCidr" || true
		else
			# fallback: accept any non-link-local IP that starts with a '10.' prefix
			# (the management network uses 10.0.0.0/8 in standard testbeds)
			echo "$i" | grep -v "^169\.254\." | grep "^10\." || true
		fi
	done
}

# Installs squid proxy on the specified remote Linux machine.
# Uses a remote flock so that when multiple e2e runner containers start in
# parallel against the same testbed only one runner reconfigures squid; the
# others acquire the lock, see squid already listening, and exit cleanly.
# All runners must still call wait_for_proxy() afterwards to block until port
# 3128 is reachable from the runner container before proceeding.
install() {
	user=$1
	ip=$2
	password=$3

	# Stage the installer script and squid config onto the gateway VM.
	sshpass -p "$password" scp $SSH_OPTS \
		"${SCRIPT_DIR}/install-squid.sh" "$user@$ip:/root/install-squid.sh"
	sshpass -p "$password" scp $SSH_OPTS \
		"${SCRIPT_DIR}/squid.conf" "$user@$ip:/root/squid.conf.new"

	# Run the install under a remote exclusive lock (/var/lock/squid-install.lock).
	# The first runner to acquire the lock does the real work; every subsequent
	# runner sees squid already listening and exits without touching anything.
	sshpass -p "$password" ssh -T $SSH_OPTS "$user@$ip" bash <<'REMOTE'
		set -euo pipefail
		(
			flock -x 200
			if netstat -tunlp 2>/dev/null | grep -q 3128; then
				echo "Squid already listening on 3128 — skipping install (another runner set it up)"
				exit 0
			fi
			/root/install-squid.sh install
			# Stop any stale process before applying the new config.
			# Failures (process not found) are intentionally ignored.
			systemctl stop squid 2>/dev/null || true
			pkill -e squid* 2>/dev/null || true
			cp /root/squid.conf.new /etc/squid/squid.conf
			cat /etc/squid/squid.conf
			/root/install-squid.sh start
			for i in $(seq 1 60); do
				if netstat -tunlp 2>/dev/null | grep -q 3128; then
					echo "Squid listening on 3128 after ${i}s"
					exit 0
				fi
				[ "$i" -eq 60 ] && { echo "ERROR: timeout waiting for squid to start"; exit 1; }
				sleep 1
			done
		) 200>/var/lock/squid-install.lock
REMOTE
}

# Blocks until port 3128 on the gateway VM is reachable from THIS runner
# container (max 120s). Called by every parallel runner after install() so
# that runners which lost the flock race still wait for the winner to finish
# starting squid before HTTP_PROXY is set and tests begin.
wait_for_proxy() {
	ip=$1
	echo "Waiting for proxy at ${ip}:3128 to be reachable from runner..."
	for i in $(seq 1 120); do
		if nc -z -w2 "${ip}" 3128 2>/dev/null; then
			echo "Proxy at ${ip}:3128 reachable after ${i}s"
			return 0
		fi
		[ "$i" -eq 120 ] && { echo "ERROR: proxy at ${ip}:3128 not reachable after 120s"; return 1; }
		sleep 1
	done
}

# stops httpd on the specified remote Linux machine
uninstall() {
	user=$1
	ip=$2
	password=$3
	sshpass -p "$password" scp $SSH_OPTS "${SCRIPT_DIR}/install-squid.sh" "$user@$ip:/root/install-squid.sh"
	sshpass -p "$password" ssh -T $SSH_OPTS "$user@$ip" /root/install-squid.sh stop
}

usage() {
	echo "./proxy.sh [install|uninstall|wait|gateway] [vCenter IP] [Management Network CIDR]"
	exit 1
}

hasGateway() {
	gatewayIp=$1
	if [[ "${gatewayIp}" == null ]]; then
		echo "Gateway IP is null or could not be found. Are you sure this is a VDS testbed?"
		exit 1
	fi
	echo "Gateway IP is ${gatewayIp}"
}

main() {
	if [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
		usage
	fi
	case $1 in
		"install")
			gatewayIp=$(find_gateway_ip $2 $3)
			hasGateway $gatewayIp
			install ${GATEWAY_VM_USERNAME} ${gatewayIp} ${GATEWAY_VM_PASSWORD}
			# Print a message that the user can copy/paste to set up their proxy.
			cat <<EOF
To set up your proxy for gce2e, source the following-
			export HTTP_PROXY=$gatewayIp:3128
			export HTTPS_PROXY=$gatewayIp:3128
EOF
			;;
		"wait")
			gatewayIp=$(find_gateway_ip $2 $3)
			hasGateway $gatewayIp
			wait_for_proxy ${gatewayIp}
			;;
		"uninstall")
			gatewayIp=$(find_gateway_ip $2 $3)
			hasGateway $gatewayIp
			uninstall ${GATEWAY_VM_USERNAME} ${gatewayIp} ${GATEWAY_VM_PASSWORD}
			;;
		"gateway")
			find_gateway_ip $2 $3
			;;
		*)
			usage
			;;
	esac
}

main "$@"
