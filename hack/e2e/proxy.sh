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
GATEWAY_VM_PASSWORD="${GATEWAY_VM_PASSWORD:-}"


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
		vmpath=$(GOVC_URL=$vc GOVC_INSECURE=$GOVC_INSECURE GOVC_USERNAME=$GOVC_USERNAME GOVC_PASSWORD=$GOVC_PASSWORD govc find / -type m -name ${vmname})
		if [[ "${vmpath}" != "" ]]; then
		echo ${vmpath}
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

# installs httpd proxy on the specified remote Linux machine with predefined configuration file
install() {
	user=$1
	ip=$2
	password=$3
	sshpass -p "$password" scp -o StrictHostKeyChecking=no ${SCRIPT_DIR}/install-squid.sh $user@$ip:/root/install-squid.sh
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip /root/install-squid.sh install
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip /root/install-squid.sh stop
	# Reason for the pkill is that on ubuntu squid systemctl restart is not reliable to refresh the process.
	# Without this the process originally started by the default package install continues and cause failure to process the updated squid config
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip pkill -e squid* || true # ignore not found
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip netstat -tunlp | grep 3128 || true  # ignore not found

	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip 'cat > /etc/squid/squid.conf' < ${SCRIPT_DIR}/squid.conf
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip 'cat /etc/squid/squid.conf'
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip /root/install-squid.sh start
	# Wait for squid to start listening on port 3128 (timeout after 60s)
	echo "Waiting for squid to start listening on port 3128..."
	for i in $(seq 1 60); do
		if sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip netstat -tunlp 2>/dev/null | grep -q 3128; then
			echo "Squid is listening on port 3128 after ${i}s"
			break
		fi
		if [ $i -eq 60 ]; then
			echo "Timeout waiting for squid to start on port 3128"
			exit 1
		fi
		sleep 1
	done
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip ps -elf | grep squid
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip netstat -tunlp | grep 3128
}

# stops httpd on the specified remote Linux machine
uninstall() {
	user=$1
	ip=$2
	password=$3
	sshpass -p "$password" scp -o StrictHostKeyChecking=no ${SCRIPT_DIR}/install-squid.sh $user@$ip:/root/install-squid.sh
	sshpass -p "$password" ssh -o StrictHostKeyChecking=no $user@$ip /root/install-squid.sh stop
}

usage() {
	echo "./proxy.sh [install|uninstall|gateway] [vCenter IP] [Management Network CIDR]"
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
