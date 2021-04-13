#!/usr/bin/env bash


set -o errexit
set -o pipefail

image="" # the full container image if any specified
binary=""
vcIp=""
vcUser="root"
vcPass='vmware'
k8sMasterIp=""
k8sMasterUser="root"
k8sMasterPass=""
sshCommArgs="-q -o PubkeyAuthentication=no -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"
backupDirectory="/etc/load-k8s-master-backups/$(date +'%Y%m%d%H%M%S')"
containerRepo="vmware"
containerFullRepo="[localhost|<IP>]:5000/${containerRepo}"
workdir=""
declare -A COMPONENTS    # Map of component name to k8s static pod manifest
declare -A BINPATHS      # Map of component name to binary path inside container
declare -A YAML_DIRS     # Map of component name to YAML
declare -A YAML_TO_COPY  # Map of YAML to copy and apply (src,dst)


COMPONENTS[vmop]="vmop.yaml"
BINPATHS[vmop]="apiserver manager"
YAML_DIRS[vmop]="/usr/lib/vmware-wcp/objects/PodVM-GuestCluster/30-vmop"  # anupamg: remove when we only use --yamlToCopy

usage() {
    echo "USAGE: $0 --component containerOrComponentName \
--binary componentBinary --vc-ip vcIp [--vc-user vcUser \
--vc-password vcPassword \
--yamlToCopy src,dst] --image componentImage \

Options:
  --binary  imagecomponentBinary  to only replace the binary inside the image
"
}

OPTS=$(getopt -o c:b:i:u:p:h --long component:,binary:,vc-ip:,vc-user:,vc-password:,yamlToCopy:,help -n 'parse-options' -- "$@")
if [[ $? != 0 ]] ; then
    echo "ERROR: Unrecognized options." >&2
    usage
    exit 1
fi
eval set -- "$OPTS"
while true; do
  case "$1" in
    -c | --component) component=$2; shift; shift;;
    -b | --binary) binary=$2; shift; shift;;
    -i | --vc-ip) vcIp=$2; shift; shift;;
    -u | --vc-user) vcUser=$2; shift; shift;;
    -p | --vc-password) vcPass=$2; shift; shift;;
    --yamlToCopy) yamlToCopy="$yamlToCopy $2"; shift; shift;;
    -h | --help ) usage ; exit 0 ;;
    -- ) shift; break ;;
    * ) usage ; exit 0 ;;
  esac
done

# Extract --yamlToCopy into YAML_TO_COPY map
for src_dst_pair in $(echo $yamlToCopy); do
    src_dst_pair_src= ; src_dst_pair_src=$(echo "$src_dst_pair" | cut -s -d "," -f 1)
    src_dst_pair_dst= ; src_dst_pair_dst=$(echo "$src_dst_pair" | cut -s -d "," -f 2)
    test -z "$src_dst_pair_src" || test -z "$src_dst_pair_dst" && {
        echo "ERROR: --yamlToCopy must have src,dst syntax."
        exit 1
    }
    YAML_TO_COPY["$src_dst_pair_src"]=$src_dst_pair_dst
done

vcCmd() {
    SSHPASS="$vcPass" sshpass -e ssh $sshCommArgs "$vcUser@$vcIp" "$@"
}

k8sMasterCmd() {
    local ipaddr=$1
    local command=$2
    SSHPASS="$k8sMasterPass" sshpass -e ssh $sshCommArgs \
        "$k8sMasterUser@$ipaddr" "$command"
}

copyToK8sMaster() {
    local ipaddr=$1
    local src=$2
    local dest=$3
    k8sMasterBackup $ipaddr $dest
    echo "$ipaddr: Copying local $src to remote $dest."
    SSHPASS="$k8sMasterPass" sshpass -e scp $sshCommArgs \
           "$src" "$k8sMasterUser@$ipaddr:$dest"
}

copyAndFlattenToK8sMaster() {
    local ipaddr=$1
    local src=$2
    local dest=$3

    if [ -d $src ]; then
        for i in $src/*; do
            copyToK8sMaster $ipaddr $i $dest/$(basename $i)
        done
    else
        copyToK8sMaster $ipaddr $src $dest
    fi
}

ensureK8sMasterPkgs() {
    local ipaddr=$1
    k8sMasterCmd $ipaddr "rpm -q tar &>/dev/null || tdnf install -y tar 1>/dev/null"
}

queryVcClusterCfg() {
    field=$1
    vcCmd "/usr/lib/vmware-wcp/decryptK8Pwd.py" | \
        while read val; do
            echo $val | awk "\$1~/$field:/ {print \$2;}"
        done
}

getAllMasterIpaddrs() {
    local ipaddr=$1
    if [[ -z $ipaddr ]]; then
        echo >&2 "WARNING: You must pass --vc-ip in order to handle multi-master SV clusters!"
        echo >&2 "WARNING: Until you do so, this  script will only modify 1 master!"
        echo ""
        return
    fi

    if ! command -v "govc" &> /dev/null; then
        echo >&2 "WARNING: You must install govc in order to handle multi-master SV clusters!"
        echo >&2 "WARNING: Until you do so, this script will only modify 1 master!"
        echo >&2 "Checkout https://github.com/vmware/govmomi/blob/master/govc/README.md"
        echo ""
        return
    fi
    export GOVC_USERNAME='administrator@vsphere.local'
    export GOVC_PASSWORD='Admin!23'
    export GOVC_INSECURE="1"
    export GOVC_URL="https://${ipaddr}/sdk"
    readarray -t vms <<<"$(govc find / -type m -name SupervisorControlPlaneVM*)"
    ipaddrs=()
    for vm in "${vms[@]}"; do
        ipaddrs+=($(govc object.collect -json "${vm}" guest.net | jq -r '.[] | .Val.GuestNicInfo[] | select(.Network == "VM Network") | .IpAddress[0]' | grep -vE "(10.0.0.10|::)" | grep .))
    done
    echo ${ipaddrs[*]}
}

k8sMasterBackup() {
    local ipaddr=$1
    local b=$2
    local bb=$backupDirectory/$2
    k8sMasterCmd $ipaddr "test -f $bb || ! test -f $b || { mkdir -p $(dirname $bb) && \
        cp $b $bb && \
        echo '$ipaddr: Backed up $b as $bb' ; }"
}

# Get the current container repo for the component in the YAML deployment file
# We support when the registry is set to "vmware/xxx:123" or the full version which can be either "localhost:5000/vmware/xxx:123"
# or "<IP>:5000/vmware/xxx:123"
# Example:
#   "vmware/xxx:123"                 -> repo=vmware
#   "localhost:5000/vmware/xxx:123"  -> repo=localhost:5000/vmware
#   "192.1.2.3:5000/vmware/xxx:123"  -> repo=192.1.2.3:5000/vmware
# Parameters: <master VM IP> <component>
getContainerRepo() {
    local ipaddr=$1
    local c=$2

    local m=${YAML_DIRS[$c]}/${COMPONENTS[$c]}
    local result=$(k8sMasterCmd ${ipaddr} "sed -n 's#[[:space:]]*image:[[:space:]]*\(\(.*:[[:digit:]]*\)\?/\?$containerRepo\)/$c:\(.*\)#\1#p' $m")
    echo "${result}"
}

# Get the full current version set for the component in the YAML deployment file
# Parameters: <master VM IP> <component> <repository>
getContainerVer() {
    local ipaddr=$1
    local c=$2
    local repo=$3

    local m=${YAML_DIRS[$c]}/${COMPONENTS[$c]}
    local result=$(k8sMasterCmd ${ipaddr} "sed -n 's#[[:space:]]*image:[[:space:]]*$repo/$c:\(.*\)#\1#p' $m")
    echo "${result}"
}

# Update the YAML file to point the containers' images to the newly installed version (dev version)
setContainerVer() {
    local ipaddr=$1
    local c=$2
    local ver=$3
    local devVer=$4
    local repo=$5

    local m=${YAML_DIRS[$c]}/${COMPONENTS[$c]}
    echo "$ipaddr: Updating container image from $repo/$component:.* to $repo/$component:$devVer in $m"
    # Blindly replace the tag after ":" as, after copying the YAML, the container version might have been updated
    k8sMasterCmd ${ipaddr} "sed -i'' -e 's#\([[:space:]]*image:[[:space:]]*\) $repo/$component:.*#\1 $repo/$component:$devVer#g' $m"
}

injectBinaries() {
    local ipaddr=$1
    local flattendir=$2
    local image=$3
    local bins=${BINPATHS[$component]}
    for i in $bins; do
        bindir=$(dirname "$flattendir/$i")
        k8sMasterCmd $ipaddr "mkdir -p $bindir"
    done

    for i in $binarySpaceDelim; do
        echo "$ipaddr: Copying $i into $bindir"
        copyToK8sMaster $ipaddr $i "$bindir/$(basename $i)"
    done
    k8sMasterCmd $ipaddr "rm -f $image && tar -C $flattendir -cf $image ."
}

addHostPrefixIfNeeded() {
    local imageName=$1
    [[ "$imageName" == "vmware/*" ]] && imageName="docker.io/${imageName}"
    echo "${imageName}"
}

test -z "$component" && {
    echo "ERROR: Missing --component arg." >&2
    exit 1
}

[ "${COMPONENTS[$component]+_}" == "" ] && {
    echo -n "ERROR: Unknown component $component, Supported: " >&2
    for c in "${!COMPONENTS[@]}" ; do echo -n "$c " >&2 ; done ; echo >&2
    exit 1
}

test -z "$binary" && {
    echo "ERROR: Missing --binary arg." >&2
    exit 1
}

test -z "$vcIp" && {
    echo "ERROR: Missing --vc-ip."
    exit 1
}

echo "Retrieving k8s master IP."
k8sMasterIp=$(queryVcClusterCfg IP 2>/dev/null) ;
echo "Retrieving k8s master password."
k8sMasterPass=$(queryVcClusterCfg PWD 2>/dev/null) ;

updateMaster() {
    local ipaddr=$1
    echo "$ipaddr: Updating"
    ensureK8sMasterPkgs $ipaddr

    # Gather repo and versions from the deployment YAML
    repoInYaml=$(getContainerRepo $ipaddr "$component" | sort | uniq)
    fullVersion=$(getContainerVer $ipaddr "$component" "$repoInYaml" | sort | uniq)
    if [[ -z "${fullVersion}" ]]; then
        echo "*** $ipaddr: ERROR: No container version retrieved for ${component}, expected ${containerRepo} or ${containerFullRepo} as the leading string for the 'image'"
        exit 1
    fi

    # Now only get the version before the "-", i.e. the version 'component:0.1-389-g1790075' will
    # give '0.1'. The version is just a string that will be the used to tag the new copied image
    ver=${fullVersion%%-*}
    # ver=$(echo $ver | sed 's/<.*>//')  # remove anything between '<' and '>'. In this case, ver equals "<wcp_appplatform_operator_ver>" means it has not been replaced, so we can assume "".
    if [[ -n "${image}" ]]; then
        obj=${image}
    else
        obj=${binary}
    fi
    # "binarySpaceDelim" is a global variable
    binarySpaceDelim=$(echo ${obj} | sed 's/,/ /g')
    devVer=${ver}
    for i in ${binarySpaceDelim}; do
        devVer=${devVer}-$(md5sum -b "$i" | cut -d ' ' -f 1)-$(date +%s)
    done
    echo "${ipaddr}: version=${ver} -> dev version=${devVer}"

    workdir="/tmp/vmware/$component"

    # Supervisor control plane VMs switched to containerd (and stopped
    # packaging docker), check if docker is available.
    noDocker=$(k8sMasterCmd $ipaddr "which docker &> /dev/null && echo false || echo true")

    layerdir="$workdir/layers"
    imagelayers="$workdir/imagelayers.tar"
    flattendir="$workdir/flattened"
    flattenmark="$workdir/.flattenedone"
    image="$workdir/image.tar"
    # Save the image. The docker image can be "vmware/", "localhost:5000/vmware" or "<ip>:5000/vmware",
    # so make sure we try what's in the YAML first (which will be  "vmware/" or "localhost:5000/vmware") and then try "<ip>:5000/vmware"
    # In the case of "ip", the "ip" will be the IP of another master
    get_image="docker images -q $repoInYaml/$component:$fullVersion"
    save_image="docker save -o $imagelayers $repoInYaml/$component:$fullVersion"
    if [ "$noDocker" == "true" ]; then
        get_image="ctr -n=k8s.io images ls -q 'name==$(addHostPrefixIfNeeded $repoInYaml/$component:$fullVersion)'";
        save_image="ctr -n=k8s.io images export $imagelayers $(addHostPrefixIfNeeded $repoInYaml/$component:$fullVersion)";
    fi
    k8sMasterCmd $ipaddr "test -f $imagelayers || { \
                echo $ipaddr: Exporting existing container image layers into $layerdir && \
                mkdir -p $layerdir && \
                if [[ \"\$($get_image 2> /dev/null)\" != \"\" ]]; then $save_image; fi && \
                tar -C $layerdir -xf $imagelayers ; }"
    k8sMasterCmd $ipaddr "test -f $flattenmark || { \
                echo 'Flattening container image layers to create a clone.' && \
                mkdir -p $flattendir && \
                if [ "$noDocker" == "true" ]; then \
                    for f in \$(jq -r '.[].Layers[]' < $layerdir/manifest.json); do \
                        tar -C $flattendir -xf $layerdir/\$f ; \
                    done ; \
                else \
                    for f in $layerdir/*/layer.tar ; do \
                        tar -C $flattendir -xf \$f ; \
                    done \
                fi && \
                touch $flattenmark ; } "
    echo "$ipaddr: Injecting binary into cloned container image."
    injectBinaries $ipaddr $flattendir $image "$binarySpaceDelim"
    echo "$ipaddr: Importing cloned container image $image as ${repoInYaml}/$component:$devVer."

    reload_image="docker import $image ${repoInYaml}/$component:$devVer"
    if [ "$noDocker" == "true" ]; then
        convert_image="/usr/local/bin/tar2oci.sh -t $image -n ${repoInYaml}/$component -v $devVer -w $workdir"
        k8sMasterCmd $ipaddr "$convert_image"
        reload_image="ctr -n=k8s.io images import $image && \
                    ctr -n=k8s.io images tag docker.io/${repoInYaml}/$component:$devVer ${repoInYaml}/$component:$devVer"  # Tag the image to remove the prefix "docker.io/", which matches the pattern in the YAML file.
    fi
    k8sMasterCmd $ipaddr "$reload_image"

    # Copy any YAML to WCP Master.
    echo "$ipaddr: Copying any YAML to WCP Master (with --yamlToCopy)"
    for src_yaml in "${!YAML_TO_COPY[@]}"; do
        copyAndFlattenToK8sMaster $ipaddr "$src_yaml" "${YAML_TO_COPY[$src_yaml]}"
    done

    # Update deployment YAML for new image, always use the same repo ${containerRepo}
    echo "$ipaddr: Updating pod manifest to point to cloned image: $ver -> $devVer"
    setContainerVer ${ipaddr} ${component} ${ver} ${devVer} ${repoInYaml}
}

MASTER_IPADDRS=$(getAllMasterIpaddrs $vcIp)
[[ -n $MASTER_IPADDRS ]] || MASTER_IPADDRS=$k8sMasterIp
echo "Masters: $MASTER_IPADDRS"

pids=()
for ipaddr in $MASTER_IPADDRS; do
    updateMaster $ipaddr & pids+=($!)
done
wait "${pids[@]}"

ipaddr=$(echo $MASTER_IPADDRS | cut -d' ' -f 1)
echo "ipaddr $ipaddr"

if [[ -n $yamlToCopy ]]; then
    echo "Applying copied YAML(s) from --yamlToCopy"
    for src_yaml in "${!YAML_TO_COPY[@]}"; do
        k8sMasterCmd $ipaddr "kubectl apply -f ${YAML_TO_COPY[$src_yaml]}"
    done
fi
echo "SUCCESS"
