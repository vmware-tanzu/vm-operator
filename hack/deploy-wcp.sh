#!/usr/bin/env bash
# Deploy VMOP and install required CRDs in the given WCP supervisor cluster
#
# Usage:
# $ deploy-wcp.sh

set -o errexit
set -o nounset
set -o pipefail
set -x

SSHCommonArgs=("-o PubkeyAuthentication=no" "-o UserKnownHostsFile=/dev/null" "-o StrictHostKeyChecking=no")

# VM service FSS
FSS_WCP_VMSERVICE_VALUE=${FSS_WCP_VMSERVICE_VALUE:-false}

# VM service v1alpha2 FSS
FSS_WCP_VMSERVICE_V1ALPHA2_VALUE=${FSS_WCP_VMSERVICE_V1ALPHA2_VALUE:-false}

# ThunderPciDevices FSS
FSS_THUNDERPCIDEVICES_VALUE=${FSS_THUNDERPCIDEVICES_VALUE:-false}

# Using VDS Networking
VSPHERE_NETWORKING_VALUE=${VSPHERE_NETWORKING_VALUE:-false}

# Upload to k8s master script
WCP_LOAD_K8S_MASTER=${WCP_LOAD_K8S_MASTER:-./hack/load-k8s-master.sh}

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

error() {
    echo "${@}" 1>&2
}

verifyEnvironmentVariables() {
    if [[ -z ${VCSA_IP:-} ]]; then
        error "Error: The VCSA_IP environment variable must be set" \
             "to point to a valid VCSA"
        exit 1
    fi

    if [[ -z ${VCSA_PASSWORD:-} ]]; then
        # Often the VCSA_PASSWORD is set to a default. The below sets a
        # common default so the user of this script does not need to set it.
        VCSA_PASSWORD="vmware"
    fi

    output=$(SSHPASS="$VCSA_PASSWORD" sshpass -e ssh "${SSHCommonArgs[@]}" \
            root@"$VCSA_IP" "/usr/lib/vmware-wcp/decryptK8Pwd.py" 2>&1)
    WCP_SA_IP=$(echo "$output" | grep -oEI "IP: (\\S)+" | cut -d" " -f2)
    WCP_SA_PASSWORD=$(echo "$output" | grep -oEI "PWD: (\\S)+" | cut -d" " -f2)

    if [[ -z ${SKIP_YAML:-} ]] ; then
        if [[ -z ${VCSA_DATACENTER:-} ]]; then
            error "Error: The VCSA_DATACENTER environment variable must be set" \
                "to point to a valid VCSA Datacenter"
            exit 1
        fi

        if [[ -z ${VCSA_CLUSTER:-} ]]; then
            error "Error: The VCSA_CLUSTER environment variable must be set" \
                "to point to a valid VCSA Cluster"
            exit 1
        fi

        VCSA_DATASTORE=${VCSA_DATASTORE:-nfs0-1}

        if [[ ${FSS_WCP_VMSERVICE_VALUE:-} != "true" ]]; then
            if [[ -z ${VCSA_CONTENT_SOURCE:-} ]]; then
                error "Error: The VCSA_CONTENT_SOURCE environment variable must be set" \
                      "to point to the ID of a valid VCSA Content Library"
                exit 1
            fi
        fi

        if [[ -z ${VCSA_WORKER_DNS:-} ]]; then
            cmd="grep WORKER_DNS /var/lib/node.cfg | cut -d'=' -f2 | sed -e 's/^[[:space:]]*//'"
            output=$(SSHPASS="$WCP_SA_PASSWORD" sshpass -e ssh "${SSHCommonArgs[@]}" \
                        "root@$WCP_SA_IP" "$cmd")
            if [[ -z $output ]]; then
                error "You did not specify env VCSA_WORKER_DNS and we couldn't fetch it from the SV cluster."
                error "Run the following on your SV node: $cmd"
                exit 1
            fi
            VCSA_WORKER_DNS=$output
        fi
    fi
}

patchWcpDeploymentYaml() {
    if [[ ${SKIP_YAML:-} != "configmap" ]]; then
        sed -i'' -E "s/<vc_pnid>/$VCSA_IP/g" "artifacts/wcp-deployment.yaml"
        sed -i'' -E "s/<cluster>/$VCSA_CLUSTER/g" "artifacts/wcp-deployment.yaml"
        sed -i'' -E "s/<datacenter>/$VCSA_DATACENTER/g" "artifacts/wcp-deployment.yaml"
        sed -i'' -E "s/ Datastore: .*/ Datastore: $VCSA_DATASTORE/" "artifacts/wcp-deployment.yaml"
        sed -i'' -E "s/<worker_dns>/$VCSA_WORKER_DNS/" "artifacts/wcp-deployment.yaml"
        sed -i'' -E "s/<content_source>/$VCSA_CONTENT_SOURCE/g" "artifacts/wcp-deployment.yaml"
    fi

    sed -i'' -E "s/<FSS_WCP_VMSERVICE_VALUE>/$FSS_WCP_VMSERVICE_VALUE/g" "artifacts/wcp-deployment.yaml"
    if grep -q "<FSS_WCP_VMSERVICE_VALUE>" artifacts/wcp-deployment.yaml; then
        echo "Failed to subst <FSS_WCP_VMSERVICE_VALUE> in artifacts/wcp-deployment.yaml"
        exit 1
    fi
    sed -i'' -E "s/<FSS_WCP_VMSERVICE_V1ALPHA2_VALUE>/$FSS_WCP_VMSERVICE_V1ALPHA2_VALUE/g" "artifacts/wcp-deployment.yaml"
    if grep -q "<FSS_WCP_VMSERVICE_V1ALPHA2_VALUE>" artifacts/wcp-deployment.yaml; then
        echo "Failed to subst <FSS_WCP_VMSERVICE_V1ALPHA2_VALUE> in artifacts/wcp-deployment.yaml"
        exit 1
    fi
    sed -i'' -E "s/<VSPHERE_NETWORKING_VALUE>/$VSPHERE_NETWORKING_VALUE/g" "artifacts/wcp-deployment.yaml"
    if grep -q "<VSPHERE_NETWORKING_VALUE>" artifacts/wcp-deployment.yaml; then
        echo "Failed to subst VSPHERE_NETWORKING_VALUE in artifacts/wcp-deployment.yaml"
        exit 1
    fi
    sed -i'' -E "s/<FSS_THUNDERPCIDEVICES_VALUE>/$FSS_THUNDERPCIDEVICES_VALUE/g" "artifacts/wcp-deployment.yaml"
    if grep -q "<FSS_THUNDERPCIDEVICES_VALUE>" artifacts/wcp-deployment.yaml; then
        echo "Failed to subst FSS_THUNDERPCIDEVICES_VALUE in artifacts/wcp-deployment.yaml"
        exit 1
    fi
    if  [[ -n ${INSECURE_TLS:-} ]]; then
        sed -i'' -E "s,InsecureSkipTLSVerify: \"?false\"?,InsecureSkipTLSVerify: \"$INSECURE_TLS\",g" "artifacts/wcp-deployment.yaml"
    fi
}

deploy() {
    local yamlArgs=""

    if [[ ${SKIP_YAML:-} != "all" ]]; then
        patchWcpDeploymentYaml
        yamlArgs+="--yamlToCopy artifacts/wcp-deployment.yaml,/usr/lib/vmware-wcp/objects/PodVM-GuestCluster/30-vmop/vmop.yaml"
        if [[ ${SKIP_YAML:-} != "vmclasses" ]]; then
            yamlArgs+=" --yamlToCopy artifacts/default-vmclasses.yaml,/usr/lib/vmware-wcp/objects/PodVM-GuestCluster/40-vmclasses/default-vmclasses.yaml"
        fi
    fi

    # shellcheck disable=SC2086
    PATH="/usr/local/opt/gnu-getopt/bin:/usr/local/bin:$PATH" \
      $WCP_LOAD_K8S_MASTER \
        --component vmop \
        --binary bin/manager \
        --vc-ip "$VCSA_IP" \
        --vc-user root \
        --vc-password "$VCSA_PASSWORD" \
        $yamlArgs
}

verifyEnvironmentVariables
deploy

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
