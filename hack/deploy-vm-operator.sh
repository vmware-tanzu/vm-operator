#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

CONFIG_YAMLS=(
    config/vmopclusterrole.yaml
    config/clusterauthdelegaterolebinding.yaml
    config/authreaderrolebinding.yaml
    config/vmoprolebinding.yaml
)

APISERVER_YAML=config/apiserver.yaml

usage () {
    echo "Usage: hack/deploy-vm-operator.sh [deploy|undeploy|redeploy]"
    exit 1
}

deploy() {
    for yaml in "${CONFIG_YAMLS[@]}"; do
        kubectl apply -f "$yaml"
    done
    kubectl apply -f "$APISERVER_YAML" --validate=false
}

undeploy() {
    kubectl delete -f "$APISERVER_YAML"

    for (( i=${#CONFIG_YAMLS[@]}-1; i >= 0; i-- )) ; do
        yaml=${CONFIG_YAMLS[$i]}
        kubectl delete -f "$yaml"
    done
}

redeploy() {
    kubectl delete -f "$APISERVER_YAML"
    kubectl apply -f "$APISERVER_YAML" --validate=false
}

while getopts ":" opt ; do
    case $opt in
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 1 ]] ; then
    usage
fi

COMMAND=$1

case $COMMAND in
    "deploy"   ) deploy ;;
    "undeploy" ) undeploy ;;
    "redeploy" ) redeploy ;;
    * ) usage
esac
