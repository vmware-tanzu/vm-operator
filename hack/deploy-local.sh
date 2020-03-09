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

$KUBECTL apply -f "$YAML"

$KUBECTL rollout restart -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
$KUBECTL rollout status -n ${VMOP_NAMESPACE} deployment ${VMOP_DEPLOYMENT}
