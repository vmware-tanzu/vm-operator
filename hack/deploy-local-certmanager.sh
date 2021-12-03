#!/usr/bin/env bash
# Deploy cert-manager to the local cluster

set -o errexit
set -o pipefail
set -o nounset
set -x

# Exit with a non-zero exit code and an error message if
# the CERT_MANAGER_URL is not set.
CERT_MANAGER_URL="${CERT_MANAGER_URL:?}"

mkdir -p artifacts

./hack/tools/bin/kustomize build \
  --load-restrictor LoadRestrictionsNone \
  "${CERT_MANAGER_URL}" \
  >artifacts/cert-manager.yaml

kubectl apply -f artifacts/cert-manager.yaml
