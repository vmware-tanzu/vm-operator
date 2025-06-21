#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

USAGE="
Usage: ${0} [[-s SV_IP,SV_IP,SV_IP [-S SV_PASSWORD]] | [-v VC_IP [-V VC_SSH_PASSWORD]] | [-T testbedInfo.json]] [-c cluster] [-t build version] image.tar

Loads the VM Operator container image to the Supervisor control plane VMs,
and restarts the deployments. The Supervisor control plane VM credential
and IPs must be either provided directly, or obtained indirectly from VC.

If you have testbedInfo.json file - either a local file or the URL - that
can be specified with the -T argument. This script will then use that file
to log into VC and obtain the necessary Supervisor info.

If you know the Supervisor CP IPs, they can be specified as a comma
separated list to the -s argument. The root password can be specified with
the -S argument; otherwise it is assumed that public key authentication
has already been configured.

If you know the VC IP, that can be specified with the -v argument. The root
password can be specified with the -V argument; otherwise it is assumed that
public key authentication has already been configured.

With either the -T or -v arguments, the first Supervisor cluster is selected
by default. To select a specific cluster, use the -c argument.

If you know the build version, it can be specified with the -t argument to
override the default 'latest' image tag.

FLAGS:
  -s Supervisor CP IPs
  -S Supervisor CP ssh password
  -v vCenter IP
  -V vCenter ssh password
  -T testbedInfo.json file or URL
  -t build version, eg '1.2.3+abcdefg+4.5.6+hijklmn'
  -c Supervisor cluster, eg 'domain-c8'
"

#########################################

# VIP host key will change as Supervisor leader migrates
COMMON_SSH_OPTS=("-o StrictHostKeyChecking=no" "-o UserKnownHostsFile=/dev/null")

SV_VIP=
SV_USERNAME="root"
SV_PASSWORD=
SV_IPS_CSL=
SV_IPS=()

VC_IP=
#VC_USERNAME=
#VC_PASSWORD=
VC_SSH_USERNAME="root"
VC_SSH_PASSWORD=

TESTBED_INFO=
SV_CLUSTER=

#########################################

function log() {
    echo "${@}"
}

function error() {
    echo "${@}" 1>&2
}

function fatal() {
    error "${@}" && exit 1
}

function vc_ssh_cmd() {
    local cmd=$1

    if [[ -n $VC_SSH_PASSWORD ]] ; then
        SSHPASS="$VC_SSH_PASSWORD" sshpass -e ssh "${COMMON_SSH_OPTS[@]}" "$VC_SSH_USERNAME@$VC_IP" "$cmd"
    else
        ssh "${COMMON_SSH_OPTS[@]}" -o PasswordAuthentication=no "$VC_SSH_USERNAME@$VC_IP" "$cmd"
    fi
}

function sv_cp_ssh_cmd() {
    local ip=$1 cmd=$2

    if [[ -n $SV_PASSWORD ]] ; then
        SSHPASS="$SV_PASSWORD" sshpass -e ssh "${COMMON_SSH_OPTS[@]}" "$SV_USERNAME@$ip" "$cmd"
    else
        ssh "${COMMON_SSH_OPTS[@]}" -o PasswordAuthentication=no "$SV_USERNAME@$ip" "$cmd"
    fi
}

function sv_cp_scp_cmd() {
    local ip=$1 file=$2

    if [[ -n $SV_PASSWORD ]] ; then
        SSHPASS="$SV_PASSWORD" sshpass -e scp -C "${COMMON_SSH_OPTS[@]}" "$file" "$SV_USERNAME@$ip:~"
    else
        scp -C "${COMMON_SSH_OPTS[@]}" -o PasswordAuthentication=no "$file" "$SV_USERNAME@$ip:~"
    fi
}

function process_testbedInfoJson() {
    local tbInfo vc

    if [[ $1 =~ ^http(s)?://.*$ ]] ; then
        tbInfo=$(curl -s "$1")
    else
        tbInfo=$(cat "$1")
    fi

    if [[ -z $tbInfo ]] ; then
        fatal "Empty testbedInfo.json?"
    fi

    vc=$(jq '.vc[0]?' <<< "$tbInfo")
    # FFS: there are different formats.
    if [[ -n $vc ]] ; then
        # VDS
        VC_IP=$(jq -r .ip <<< "$vc")
        #VC_USERNAME=$(jq -r .vimUsername <<< "$vc")
        #VC_PASSWORD=$(jq -r .vimPassword <<< "$vc")
        VC_SSH_USERNAME=$(jq -r .username <<<" $vc")
        VC_SSH_PASSWORD=$(jq -r .password <<< "$vc")
    else
        # NSX-T
        vc=$(jq -c '.vc["1"]' <<< "$tbInfo")

        VC_IP=$(jq -r .ip <<< "$vc")
        #VC_USERNAME=$(jq -r .username <<< "$vc")
        #VC_PASSWORD=$(jq -r .password <<< "$vc")
        VC_SSH_USERNAME="root"
        VC_SSH_PASSWORD=$(jq -r .root_password <<< "$vc")
    fi

    # TODO: For some envs we need to use the jumphost to reach the SV.
}

function sv_get_vip_and_password() {
    local k8sPwd sv pattern

    log "Getting Supervisor VIP and password from VC..."

    k8sPwd=$(vc_ssh_cmd "/usr/lib/vmware-wcp/decryptK8Pwd.py")

    if [[ -z $SV_CLUSTER ]] ; then
        pattern='^Cluster:'
    else
        pattern="^Cluster: ${SV_CLUSTER}:"
    fi

    sv=$(grep -m1 -A3 -e "$pattern" <<< "$k8sPwd" || true)
    if [[ -z $sv ]] ; then
        fatal "No Supervisor cluster found"
    fi

    SV_VIP=$(echo "$sv" | awk '/^IP:/ {print $2}')
    SV_PASSWORD=$(echo "$sv" | awk '/^PWD:/ {print $2}')

    log "Supervisor VIP: $SV_VIP Password: $SV_PASSWORD"
}

function sv_get_cp_ips() {
    local cmd

    # We don't populate the Nodes' ExternalIP field so get it this way.
    cmd="kubectl get cm -n kube-system wcp-network-config -o jsonpath='{.data.api_servers_management_ips}'"

    sv_cp_ssh_cmd "$SV_VIP" "$cmd"
}

function sv_copy_and_load_image() {
    local svip=$1 image=$2

    sv_cp_scp_cmd "$svip" "$image"
    sv_cp_ssh_cmd "$svip" "ctr -n=k8s.io images import ${image##*/}"
    #sv_cp_ssh_cmd "$svip" "ctr -n k8s.io images ls | grep vmop"

    log "$svip: copied and loaded image"
}

function sv_update_deployment_image() {
    local image=$1 ip cmd

    ip=${SV_VIP:-${SV_IPS[0]}}

    cmd="kubectl set image -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager manager='$image' && \
         kubectl set image -n vmware-system-vmop deployment/vmware-system-vmop-web-console-validator web-console-validator='$image'"

    sv_cp_ssh_cmd "$ip" "$cmd"
}

function sv_restart_vmop_deployment() {
    local ip

    ip=${SV_VIP:-${SV_IPS[0]}}

    # First time "set image" will bounce them but whatever, just always do this.
    sv_cp_ssh_cmd "$ip" "kubectl rollout restart deployment -n vmware-system-vmop"
}

#########################################

while getopts ":hc:s:S:v:V:T:t:" opt ; do
    case $opt in
        h)
            echo "$USAGE"
            exit 0
            ;;
        c)
            SV_CLUSTER=$OPTARG
            ;;
        s)
            SV_IPS_CSL=$OPTARG
            ;;
        S)
            SV_PASSWORD=$OPTARG
            ;;
        v)
            VC_IP=$OPTARG
            ;;
        V)
            VC_SSH_PASSWORD=$OPTARG
            ;;
        T)
            TESTBED_INFO=$OPTARG
            ;;
        t)
            BUILD_VERSION=$OPTARG
            ;;
        *)
            fatal "$USAGE"
            ;;
    esac
done

shift $((OPTIND-1))

if [[ $# -ne 1 ]] ; then
    fatal "$USAGE"
fi

# Easiest tag to just assume. Maybe revist if we combine "podman build", "podman save",
# and this script into one.
BUILD_VERSION="${BUILD_VERSION:-latest}"
IMAGE_REF=${IMAGE_REF:-"localhost/vmoperator-controller:${BUILD_VERSION}"}

IMAGE=$1

if [[ ! -r $IMAGE ]] ; then
    fatal "Container image $IMAGE does not exist"
fi

if [[ -z $SV_IPS_CSL ]] ; then
    if [[ -z $VC_IP ]] ; then
        if [[ -z $TESTBED_INFO ]] ; then
            fatal "No testbedInfo.json: either vCenter or Supervisor CP IPs is required"
        fi

        process_testbedInfoJson "$TESTBED_INFO"
    fi

    sv_get_vip_and_password
    SV_IPS_CSL=$(sv_get_cp_ips)
fi

mapfile -t SV_IPS <<< "$(echo "$SV_IPS_CSL" | tr "," "\n")"
if [[ ${#SV_IPS[@]} -eq 0 ]] ; then
    fatal "Could not parse Supervisor IPs list: $SV_IPS_CSL"
fi

log "Supervisor CP IPs: ${SV_IPS[*]}"

# TODO: Our image is ~30MB compressed, which isn't too bad to upload 3
# times but could use the first SV CP as the source for the other 2.
pids=()
for ip in "${SV_IPS[@]}" ; do
    sv_copy_and_load_image "$ip" "$IMAGE" &
    pids+=($!)
done
wait "${pids[@]}"

log "Updating deployment image to $IMAGE_REF"
sv_update_deployment_image "$IMAGE_REF"
sv_restart_vmop_deployment

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
