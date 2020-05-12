#!/usr/bin/env bash
# Deploy VMOP to the given cluster
#
# Usage:
# $ deploy-local.sh <yaml>

set -o errexit
set -o pipefail
set -o nounset

YAML=$1
KUBECTL="kubectl"

VMOP_NAMESPACE="vmware-system-vmop"
VMOP_DEPLOYMENT="vmware-system-vmop-controller-manager"

ARTIFACTS_DIR="artifacts"
DEFAULT_VMCLASSES_YAML="${ARTIFACTS_DIR}/default-vmclasses.yaml"

$KUBECTL apply -f "$YAML"

$KUBECTL rollout restart -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
$KUBECTL rollout status -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}

# Hack that retries applying the default VM Classes until the
# validating webhook is available.
VMOP_VMCLASSES_ATTEMPTS=0
while true ; do
    kubectl apply -f "${DEFAULT_VMCLASSES_YAML}" && break
    VMOP_VMCLASSES_ATTEMPTS=$((VMOP_VMCLASSES_ATTEMPTS+1))
    if [[ $VMOP_VMCLASSES_ATTEMPTS -ge 60 ]] ; then
	log "Failed to apply default VM Classes"
	exit 1
    fi
    echo "Cannot create default VM Classes. Trying again."
    sleep "5s"
done
