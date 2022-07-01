#!/usr/bin/env bash

# Run the unit tests.
# If an argument is given, the coverage will be recorded:
# test-unit.sh [<coverage file>]

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
NO_RACE_COVERAGE_FILE="${PWD}/no_race_unit_cover.out"

COVER_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
COV_OPTS=$(join_packages_for_cover "${COVER_PKGS[@]}")

TEST_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)

# Packages that cannot be tested with "-race" due to dependency races
NO_RACE_TEST_PKGS=(
  "./pkg/vmprovider/providers/vsphere/client"
)

ENV_GOFLAGS=()
NO_RACE_ENV_GOFLAGS=()

# The first argument is the name of the coverage file to use.
if [[ -n ${COVERAGE_FILE} ]]; then
    ENV_GOFLAGS+=("-coverprofile=${COVERAGE_FILE}" "-coverpkg=${COV_OPTS}" "-covermode=atomic")
    NO_RACE_ENV_GOFLAGS+=("-coverprofile=${NO_RACE_COVERAGE_FILE}" "-coverpkg=${COV_OPTS}" "-covermode=atomic")
fi

GINKGO_FLAGS=()

if [[ -n ${JOB_NAME:-} ]]; then
    # Disable color output on Jenkins.
    GINKGO_FLAGS+=("-ginkgo.noColor")
fi

# Run unit tests
# go test: -race requires cgo
# shellcheck disable=SC2046
CGO_ENABLED=1 go test -v -race -count=1 "${ENV_GOFLAGS[@]}" \
    $(join_packages_for_tests "${TEST_PKGS[@]}") \
    "${GINKGO_FLAGS[@]}" \
    -- \
    -enable-unit-tests=true \
    -enable-integration-tests=false

# Run certain unit tests without -race
# shellcheck disable=SC2046
go test -v -count=1 "${NO_RACE_ENV_GOFLAGS[@]}" \
    $(join_packages_for_tests "${NO_RACE_TEST_PKGS[@]}") \
    "${GINKGO_FLAGS[@]}" \
    -- \
    -enable-unit-tests=true \
    -enable-integration-tests=false

# Merge the coverage files.
if [[ -n ${COVERAGE_FILE} ]]; then
    TMP_COVERAGE_FILE=$(mktemp)
    mv "${COVERAGE_FILE}" "${TMP_COVERAGE_FILE}"
    gocovmerge "${TMP_COVERAGE_FILE}" "${NO_RACE_COVERAGE_FILE}" > "${COVERAGE_FILE}"
    rm -f "${TMP_COVERAGE_FILE}" "${NO_RACE_COVERAGE_FILE}"
fi
