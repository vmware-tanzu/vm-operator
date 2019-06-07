#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

DEPLOYMENT_YAML=artifacts/local-deployment.yaml
REDEPLOYMENT_YAML=artifacts/local-redeployment.yaml

usage () {
    echo "Usage: $(basename $0) [deploy|undeploy|redeploy]"
    exit 1
}

deploy() {
    kubectl kustomize config/local > "$DEPLOYMENT_YAML"
    kubectl apply -f "$DEPLOYMENT_YAML"
}

undeploy() {
    kubectl delete -f "$DEPLOYMENT_YAML"
}

redeploy() {
    kubectl kustomize config/local-redeploy > "$REDEPLOYMENT_YAML"
    kubectl delete -f "$REDEPLOYMENT_YAML"
    kubectl apply -f "$REDEPLOYMENT_YAML" --validate=false
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

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
