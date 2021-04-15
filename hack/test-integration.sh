#!/usr/bin/env bash

# Run the integration tests.
# If an argument is given, the coverage will be recorded:
# test-integration.sh [<coverage file>]

set -o errexit
set -o nounset
set -o pipefail
set -x

function join_packages { local IFS=","; echo "$*"; }

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

COVERAGE_FILE="${1-}"
INTEGRATION_COVERAGE_FILE="$(pwd)/int.cover.out"
ENVIRONMENT_COVERAGE_FILE="$(pwd)/env.cover.out"

# shellcheck disable=SC1091
source hack/ensure-go.sh

packages=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
COV_OPTS=$(join_packages "${packages[@]}")

INT_GOFLAGS=()
ENV_GOFLAGS=()

# The first argument is the name of the coverage file to use.
if [[ -n ${COVERAGE_FILE} ]]; then
    INT_GOFLAGS=("-coverprofile=${INTEGRATION_COVERAGE_FILE}" "-coverpkg=${COV_OPTS}")
    ENV_GOFLAGS=("-coverprofile=${ENVIRONMENT_COVERAGE_FILE}" "-coverpkg=${COV_OPTS}")
fi

# Run the package integration tests.
# go test: -race requires cgo
CGO_ENABLED=1 go test -v -race -p 1 -count=1 "${INT_GOFLAGS[@]}" -tags=integration ./pkg/...

# Run integration tests with new framework
# go test: -race requires cgo
CGO_ENABLED=1 go test -v -race -p 1 -count=1 "${ENV_GOFLAGS[@]}" \
           ./controllers/contentsource \
           ./controllers/infracluster \
           ./controllers/infraprovider \
           ./controllers/providerconfigmap \
           ./controllers/virtualmachine \
           ./controllers/virtualmachineclass \
           ./controllers/virtualmachineimage \
           ./controllers/virtualmachineservice \
           ./controllers/virtualmachinesetresourcepolicy \
           ./controllers/volume \
	   ./webhooks/... \
           -- -enable-integration-tests -enable-unit-tests=false

# Merge the coverage files.
if [[ -n ${COVERAGE_FILE} ]]; then
    touch "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}"
    gocovmerge "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}" > "${COVERAGE_FILE}"
    rm -f "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}"
fi
