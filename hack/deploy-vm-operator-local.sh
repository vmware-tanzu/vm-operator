#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

DEPLOYMENT_YAML=artifacts/local-deployment.yaml
VMCLASSES_YAML=artifacts/default-vmclasses.yaml
REDEPLOYMENT_YAML=artifacts/local-redeployment.yaml
APISERVICE_NAME="v1alpha1.vmoperator.vmware.com"
LOCAL_CONFIG=config/local

usage () {
    echo "Usage: $(basename $0) [deploy|deploy-vcsim|undeploy|redeploy]"
    exit 1
}

deploy-vcsim() {
    LOCAL_CONFIG=config/local-vcsim
    deploy
    kubectl apply -f "$LOCAL_CONFIG/sc.yaml"
}

deploy() {
    kubectl kustomize "$LOCAL_CONFIG" > "$DEPLOYMENT_YAML"
    kubectl kustomize config/virtualmachineclasses > "$VMCLASSES_YAML"

    restart=""
    if kubectl get deployment -n default vmoperator-controller >/dev/null 2>&1 ; then
        restart=1
    fi

    kubectl apply -f "$DEPLOYMENT_YAML"

    if [[ -n $restart ]] ; then
        kubectl rollout restart -n default deployment vmoperator-apiserver
        kubectl rollout restart -n default deployment vmoperator-controller

        kubectl rollout status -n default deployment vmoperator-apiserver
        kubectl rollout status -n default deployment vmoperator-controller
    fi

    # wait for the aggregated api server to come up so we can install the VM classes
    maxAttempts=100
    numAttempts=0

    conditionType=$(kubectl get apiservices $APISERVICE_NAME -o=jsonpath='{.status.conditions[0].type}')
    status=$(kubectl get apiservices $APISERVICE_NAME -o=jsonpath='{.status.conditions[0].status}')

    until [ "$conditionType" == "Available" ] && [ "$status" == "True" ]; do
        numAttempts=$((numAttempts+1))
        if [ $numAttempts == $maxAttempts ]; then
            echo "APIserver pod did not start on time"
            command="kubectl get pod --all-namespaces"
            echo "$command" && eval "$command"
            return 1
        fi
        echo "APIserver pod not ready yet. Trying again"
        sleep 2s
        conditionType=$(kubectl get apiservices $APISERVICE_NAME -o=jsonpath='{.status.conditions[0].type}')
        status=$(kubectl get apiservices $APISERVICE_NAME -o=jsonpath='{.status.conditions[0].status}')
    done

    kubectl apply -f "$VMCLASSES_YAML"
}

undeploy() {
    kubectl delete -f "$VMCLASSES_YAML" --ignore-not-found
    kubectl delete -f "$DEPLOYMENT_YAML" --ignore-not-found
}

# redeply does not redeploy the vmclasses since we do not expect them to change
redeploy() {
    kubectl kustomize config/local-redeploy > "$REDEPLOYMENT_YAML"
    kubectl delete -f "$REDEPLOYMENT_YAML" --ignore-not-found
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
    "deploy-vcsim" ) deploy-vcsim ;;
    "undeploy" ) undeploy ;;
    "redeploy" ) redeploy ;;
    * ) usage
esac

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
