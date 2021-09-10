#!/usr/bin/env bash

# Run the integration tests.
# If an argument is given, the coverage will be recorded:
# test-integration.sh [<coverage file>]

set -o errexit
set -o nounset
set -o pipefail
set -x

function join_packages_for_cover { local IFS=","; echo "$*"; }
function join_packages_for_tests { local IFS=" "; echo "$*"; }

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

COVERAGE_FILE="${1-}"
INTEGRATION_COVERAGE_FILE="$(pwd)/int.cover.out"
ENVIRONMENT_COVERAGE_FILE="$(pwd)/env.cover.out"

COVER_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
COV_OPTS=$(join_packages_for_cover "${COVER_PKGS[@]}")

# Packages tested with the old test framework.
TEST_PKGS=(
  "./pkg/vmprovider/..."
)

# Packages tested with the new test framework.
NEW_TEST_PKGS=(
  "./controllers/contentsource"
  "./controllers/infracluster"
  "./controllers/infraprovider"
  "./controllers/providerconfigmap"
  "./controllers/virtualmachine"
  "./controllers/virtualmachineclass"
  "./controllers/virtualmachineimage"
  "./controllers/virtualmachineservice"
  "./controllers/virtualmachinesetresourcepolicy"
  "./controllers/volume"
  "./webhooks/..."
)

INT_GOFLAGS=()
ENV_GOFLAGS=()

# The first argument is the name of the coverage file to use.
if [[ -n ${COVERAGE_FILE} ]]; then
    INT_GOFLAGS=("-coverprofile=${INTEGRATION_COVERAGE_FILE}" "-coverpkg=${COV_OPTS}")
    ENV_GOFLAGS=("-coverprofile=${ENVIRONMENT_COVERAGE_FILE}" "-coverpkg=${COV_OPTS}")
fi

# Run the package integration tests.
# go test: -race requires cgo
# shellcheck disable=SC2046
CGO_ENABLED=1 go test -v -race -p 1 -count=1 "${INT_GOFLAGS[@]}" \
  -tags=integration \
  $(join_packages_for_tests "${TEST_PKGS[@]}")

# Run integration tests with new framework
# go test: -race requires cgo
# shellcheck disable=SC2046
CGO_ENABLED=1 go test -v -race -p 1 -count=1 "${ENV_GOFLAGS[@]}" \
           $(join_packages_for_tests "${NEW_TEST_PKGS[@]}") \
           -- \
           -enable-integration-tests \
           -enable-unit-tests=false

# Merge the coverage files.
if [[ -n ${COVERAGE_FILE} ]]; then
    touch "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}"
    gocovmerge "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}" > "${COVERAGE_FILE}"
    rm -f "${ENVIRONMENT_COVERAGE_FILE}" "${INTEGRATION_COVERAGE_FILE}"
fi
