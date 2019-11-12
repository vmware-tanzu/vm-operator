#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

YAML= # bryanv: Not used (yet?)
SSHCommonArgs=" -o PubkeyAuthentication=no -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no "

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

    if [[ -z ${VCSA_DATACENTER:-} ]]; then
        echo "Error: The VCSA_DATACENTER environment variable must be set" \
             "to point to a valid VCSA Datacenter"
        exit 1
    fi

    VCSA_DATASTORE=${VCSA_DATASTORE:-nfs0-1}

    if [[ -z ${VCSA_IP:-} ]]; then
        echo "Error: The VCSA_IP environment variable must be set" \
             "to point to a valid VCSA"
        exit 1
    fi

    VCSA_PASSWORD=${VCSA_PASSWORD:-vmware}

    output=$(SSHPASS="$VCSA_PASSWORD" sshpass -e ssh $SSHCommonArgs \
                root@$VCSA_IP "/usr/lib/vmware-wcp/decryptK8Pwd.py" 2>&1)
    WCP_SA_IP=$(echo $output | grep -oEI "IP: (\S)+" | cut -d" " -f2)
    WCP_SA_PASSWORD=$(echo $output | grep -oEI "PWD: (\S)+" | cut -d" " -f2)

    if ! ping -c 3 -i 0.25 $WCP_SA_IP > /dev/null 2>&1 ; then
        echo "Error: Could not access WCP Supervisor Cluster API Server at" \
             "$WCP_SA_IP"
        exit 1
    fi

    if [[ -z ${VCSA_WORKER_DNS:-} ]]; then
        cmd="grep WORKER_DNS /var/lib/node.cfg | cut -d'=' -f2 | sed -e 's/^[[:space:]]*//'"
        output=$(SSHPASS="$WCP_SA_PASSWORD" sshpass -e ssh $SSHCommonArgs \
                    root@$WCP_SA_IP "$cmd")
        if [[ -z $output ]]; then
            echo "You did not specify env VCSA_WORKER_DNS and we couldn't fetch it from the SV cluster."
            echo "Run the following on your SV node: $cmd"
            exit 1
        fi
        VCSA_WORKER_DNS=$output
    fi
}

patchWcpDeploymentYaml() {
    if [[ -f  "artifacts/wcp-deployment.yaml" ]]; then
        sed -i'' "s,<vc_pnid>,$VCSA_IP,g" "artifacts/wcp-deployment.yaml"
        sed -i'' "s,<datacenter>,$VCSA_DATACENTER,g" "artifacts/wcp-deployment.yaml"
        sed -i'' "s, Datastore: .*, Datastore: $VCSA_DATASTORE," "artifacts/wcp-deployment.yaml"
        sed -i'' "s,<worker_dns>,$VCSA_WORKER_DNS," "artifacts/wcp-deployment.yaml"
    fi
}

deploy() {
    patchWcpDeploymentYaml
    PATH="/usr/local/opt/gnu-getopt/bin:/usr/local/bin:$PATH" \
        $WCP_LOAD_K8S_MASTER \
        --component vmop \
        --binary bin/linux/apiserver,bin/linux/manager \
        --vc-ip $VCSA_IP \
        --vc-user root \
        --vc-password $VCSA_PASSWORD \
        --yamlToCopy artifacts/wcp-deployment.yaml,/usr/lib/vmware-wcp/objects/PodVM-GuestCluster/30-vmop/vmop.yaml \
        --yamlToCopy artifacts/default-vmclasses.yaml,/usr/lib/vmware-wcp/objects/PodVM-GuestCluster/40-vmclasses/default-vmclasses.yaml
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
