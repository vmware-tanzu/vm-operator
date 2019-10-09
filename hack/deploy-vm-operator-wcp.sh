#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

YAML= # bryanv: Not used (yet?)

usage () {
    cat <<EOM
Usage: $(basename $0) [deploy|undeploy|redeploy] [-Y yaml]
    -Y: k8s yaml deployment configuration
EOM
    exit 1
}

verifyEnvironmentVariables() {
    if [[ -z ${WCP_LOAD_K8S_MASTER:-} ]]; then
        echo "Error: The WCP_LOAD_K8S_MASTER environment variable must be set" \
             "to point to a copy of load-k8s-master"
        exit 1
    fi

    if [[ ! -x $WCP_LOAD_K8S_MASTER ]]; then
        echo "Error: Could not find the load-k8s-master script. Please" \
             "verify the environment variable WCP_LOAD_K8S_MASTER is set" \
             "properly. The load-k8s-master script is found at" \
             "bora/vpx/wcp/support/load-k8s-master in a bora repo."
        exit 1
    fi

    if [[ -n ${VCSA_IP:-} ]]; then
        if [[ -z ${VCSA_PASSWORD:-} ]]; then
            # Often the VCSA_PASSWORD is set to a default. The below sets a
            # common default so the user of this script does not need to set it.
            VCSA_PASSWORD="vmware"
        fi
        output=$(SSHPASS="$VCSA_PASSWORD" sshpass -e ssh -o \
                 UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
                 root@$VCSA_IP "/usr/lib/vmware-wcp/decryptK8Pwd.py" 2>&1)
        WCP_SA_IP=$(echo $output | grep -oEI "IP: (\S)+" | cut -d" " -f2)
        WCP_SA_PASSWORD=$(echo $output | grep -oEI "PWD: (\S)+" | cut -d" " -f2)
    fi

    if [[ -z ${WCP_SA_IP:-} ]]; then
        echo "Error: The WCP_SA_IP environment variable must be set to the" \
             "WCP Supervisor Cluster API Server's IP address"
        exit 1
    fi

    if [[ -z ${WCP_SA_PASSWORD:-} ]]; then
        echo "Error: The WCP_SA_PASSWORD environment variable must be set to" \
             "the WCP Supervisor Cluster API Server's root password"
        exit 1
    fi

    if ! ping -c 3 -i 0.25 $WCP_SA_IP > /dev/null 2>&1 ; then
        echo "Error: Could not access WCP Supervisor Cluster API Server at" \
             "$WCP_SA_IP"
        exit 1
    fi
}

deploy() {
    PATH="/usr/local/opt/gnu-getopt/bin:/usr/local/bin:$PATH" \
        $WCP_LOAD_K8S_MASTER \
        --component vmop \
        --binary bin/linux/apiserver,bin/linux/manager \
        --k8s-master-ip $WCP_SA_IP \
        --k8s-master-password $WCP_SA_PASSWORD \
        --yamlToApply artifacts/default-vmclasses.yaml,artifacts/wcp-deployment.yaml \
        --yamlDestination /usr/lib/vmware-wcp/objects/PodVM-GuestCluster/30-vmop
}

undeploy() {
   echo "Not implemented"
   exit 1
}

redeploy() {
   echo "Not implemented"
   exit 1
}

while getopts ":Y:" opt ; do
    case $opt in
        "Y" ) YAML=$OPTARG ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 1 ]] ; then
    usage
fi

COMMAND=$1

case $COMMAND in
    "deploy"   ) verifyEnvironmentVariables; deploy ;;
    "undeploy" ) verifyEnvironmentVariables; undeploy ;;
    "redeploy" ) verifyEnvironmentVariables; redeploy ;;
    * ) usage
esac

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
