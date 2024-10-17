#!/usr/bin/env bash
# Deploy VMOP to the given cluster
#
# Usage:
# $ deploy-local.sh <deploy_yaml> [vmclasses_yaml]

set -o errexit
set -o pipefail
set -o nounset
set -x

# VERIFY_MANIFESTS is set to a non-empty value if the only thing this file
# should be doing is verifying the YAML can be successfully applied to a Kind
# cluster. When this is set, we do not wait to verify that deployments,
# webhooks, pods, etc. are online. The only thing about which we care is if
# "kubectl apply" succeeded.
VERIFY_MANIFESTS="${VERIFY_MANIFESTS:-}"

YAML="${1}"
KUBECTL="kubectl"

VMOP_NAMESPACE="vmware-system-vmop"
VMOP_DEPLOYMENT="vmware-system-vmop-controller-manager"

DEPLOYMENT_EXISTS=""
if [ -z "${VERIFY_MANIFESTS:-}" ]; then
  if $KUBECTL get deployment -n ${VMOP_NAMESPACE} ${VMOP_DEPLOYMENT} >/dev/null 2>&1; then
    DEPLOYMENT_EXISTS=1
  fi
fi

# Check to see if cert-manager exists.
CERTMANAGER_NAMESPACE="${CERTMANAGER_NAMESPACE:-cert-manager}"
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
  echo $'\nSleeping for 1m - waiting for cert-manager webhooks to be initialized\n'
  sleep 60
fi

if [ -z "${VERIFY_MANIFESTS:-}" ]; then
  # Hack to reduce the number of replicas deployed from 3 to 1
  # when deploying onto a single node kind cluster.
  NODE_COUNT=$(kubectl get node --no-headers 2>/dev/null | wc -l)
  if [ "$NODE_COUNT" -eq 1 ]; then
    sed -i -e 's/replicas: 3/replicas: 1/g' "$YAML"
    # remove the generated '-e' file on Mac
    rm -f "$YAML-e"
  fi
fi

# Apply the infrastructure YAML.
$KUBECTL apply -f "$YAML"

# If VERIFY_MANIFESTS is set then we can go ahead and exit.
if [ -n "${VERIFY_MANIFESTS:-}" ]; then
  exit 0
fi

if [[ -n $DEPLOYMENT_EXISTS ]]; then
  $KUBECTL rollout restart -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
  $KUBECTL rollout status -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
fi

until $KUBECTL wait --for=condition=Ready -n vmware-system-vmop cert/vmware-system-vmop-serving-cert; do
  sleep 1
done

if [[ $# -eq 2 ]]; then
  VMCLASSES_YAML="${2}"
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
