#!/usr/bin/env bash
# Deploy cert-manager to the local cluster

set -o errexit
set -o pipefail
set -o nounset
set -x

CERT_MANAGER_URL="${CERT_MANAGER_URL:-https://github.com/cert-manager/cert-manager/releases/download/v1.16.1/cert-manager.yaml}"
CERT_MANAGER_YAML="artifacts/cert-manager.yaml"

mkdir -p "$(dirname "${CERT_MANAGER_YAML}")"

curl -Lo "${CERT_MANAGER_YAML}" "${CERT_MANAGER_URL}"

kubectl apply -f "${CERT_MANAGER_YAML}"
