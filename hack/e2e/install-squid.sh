#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -x

# Function to ensure that the "squid" package is installed
ensure_squid_package() {
    if [ -f "/etc/os-release" ]; then
        source "/etc/os-release"
        if [[ $ID == "ubuntu" ]]; then
            if [ "$(dpkg-query -W -f='${Status}' squid 2>/dev/null | grep -c "ok installed")" -eq 0 ]; then
                echo "Installing Squid package..."
                dpkg --configure -a # to fix the imporoperly configured packages in the vm (as left in the appliance image) 
                apt-get install -y squid
            else
                echo "Squid package is already installed."
            fi
        elif [[ $ID == "photon" ]]; then
            if ! rpm -q squid >/dev/null 2>&1; then
                echo "Installing Squid package..."
                tdnf install -y squid --nogpgcheck
            else
                echo "Squid package is already installed."
            fi
        else
            echo "Unsupported operating system. Exiting..."
            exit 1
        fi
    else
        echo "Unable to detect operating system. Exiting..."
        exit 1
    fi
}

# Function to interact with the squid service
squid_service_action() {
    echo "${1}ing Squid service..."
    systemctl ${1} squid
}

usage() {
	echo "./install-squid.sh [install|start|restart|stop]"
	exit 1
}

main() {
	if [ -z "$1" ]; then
		usage
	fi
	case $1 in
		"install")
            ensure_squid_package
			;;
		"start"|"stop"|"restart")
			squid_service_action $1
			;;
		*)
			usage
			;;
	esac
}

main "$@"
