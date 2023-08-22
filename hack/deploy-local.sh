#!/usr/bin/env bash
# Deploy VMOP to the given cluster
#
# Usage:
# $ deploy-local.sh <deploy_yaml> [vmclasses_yaml]

set -o errexit
set -o pipefail
set -o nounset

YAML=$1
KUBECTL="kubectl"

VMOP_NAMESPACE="vmware-system-vmop"
VMOP_DEPLOYMENT="vmware-system-vmop-controller-manager"

DEPLOYMENT_EXISTS=""
if $KUBECTL get deployment -n ${VMOP_NAMESPACE} ${VMOP_DEPLOYMENT} >/dev/null 2>&1; then
  DEPLOYMENT_EXISTS=1
fi

# Deploy and check cert-manager
CERTMANAGER_NAMESPACE="vmware-system-cert-manager"
CERTMANAGER_DEPLOYMENTS=(
  cert-manager
  cert-manager-cainjector
  cert-manager-webhook
)

CERTMAN_EXISTS=""
if $KUBECTL get deployment -n "${CERTMANAGER_NAMESPACE}" "${CERTMANAGER_DEPLOYMENTS[0]}" >/dev/null 2>&1 ; then
  CERTMAN_EXISTS="exists"
fi
if [[ -z $CERTMAN_EXISTS ]]; then
  ./hack/deploy-local-certmanager.sh
  for dep in "${CERTMANAGER_DEPLOYMENTS[@]}"; do
    $KUBECTL rollout status -n "${CERTMANAGER_NAMESPACE}" deployment "${dep}"
  done

  # TODO Find a better way to wait for this...
  echo $'\nSleeping for 60s - waiting for webhooks to be initialized\n'
  sleep 60
fi

# Hack to reduce the number of replicas deployed from 3 to 1
# when deploying onto a single node kind cluster.
NODE_COUNT=$(kubectl get node --no-headers 2>/dev/null | wc -l)
if [ "$NODE_COUNT" -eq 1 ]; then
  sed -i -e 's/replicas: 3/replicas: 1/g' "$YAML"
  # remove the generated '-e' file on Mac
  rm -f "$YAML-e"
fi

FSS_V1A2=$(yq '.spec.template.spec.containers[]| select(.name == "manager") | .env[] | select(.name == "FSS_WCP_VMSERVICE_V1ALPHA2") | .value' "$YAML")
if [ "${FSS_V1A2}" = "false" ]; then \
  yq -i 'del(.spec.versions[] | select(.name == "v1alpha2"))' "$YAML"
  yq -i 'del(.webhooks[] | select(.name == "*v1alpha2*"))' "$YAML"
fi;

$KUBECTL apply -f "$YAML"

if [[ -n $DEPLOYMENT_EXISTS ]]; then
  $KUBECTL rollout restart -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
  $KUBECTL rollout status -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
fi

until $KUBECTL wait --for=condition=Ready -n vmware-system-vmop cert/vmware-system-vmop-serving-cert; do
  sleep 1
done

if [[ $# -eq 2 ]]; then
  VMCLASSES_YAML=$2
  # Hack that retries applying the default VM Classes until the
  # validating webhook is available.
  VMOP_VMCLASSES_ATTEMPTS=0
  while true; do
    $KUBECTL apply -f "${VMCLASSES_YAML}" && break
    VMOP_VMCLASSES_ATTEMPTS=$((VMOP_VMCLASSES_ATTEMPTS + 1))
    if [[ $VMOP_VMCLASSES_ATTEMPTS -ge 60 ]]; then
      echo "Failed to apply default VM Classes"
      exit 1
    fi
    echo "Cannot create default VM Classes. Trying again."
    sleep 5
  done
fi
