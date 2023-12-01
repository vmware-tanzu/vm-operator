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

COVERAGE_FILE="${1:-}"
if [ -n "${COVERAGE_FILE}" ]; then
  COVERAGE_FILE_NORACE="${COVERAGE_FILE}.norace"
  API_COVERAGE_FILE="api.${COVERAGE_FILE}"
fi

COVER_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
COVER_OPTS=$(join_packages_for_cover "${COVER_PKGS[@]}")

TEST_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)

# Packages that cannot be tested with "-race" due to dependency races
TEST_PKGS_NORACE=(
  "./pkg/vmprovider/providers/vsphere/client"
)

GINKGO_FLAGS=()
GO_TEST_FLAGS=("-v")

if [ -n "${JOB_NAME:-}" ]; then
  # Disable color output on Jenkins.
  GINKGO_FLAGS+=("-ginkgo.noColor")
fi

# GitHub actions store the test cache, so do not force tests
# to re-run when running as part of a GitHub action.
if [ -z "${GITHUB_ACTION:-}" ]; then
  GO_TEST_FLAGS+=("-count=1")
fi

GO_TEST_FLAGS+=("-race")

# Test flags for api module
API_TEST_FLAGS=("${GO_TEST_FLAGS[@]}")

# The first argument is the name of the coverage file to use.
if [ -n "${COVERAGE_FILE}" ]; then
  API_TEST_FLAGS+=("-coverpkg=github.com/vmware-tanzu/vm-operator/api/..." "-covermode=atomic")
  API_TEST_FLAGS+=("-coverprofile=${API_COVERAGE_FILE}")
  GO_TEST_FLAGS+=("-coverpkg=${COVER_OPTS}" "-covermode=atomic")
  GO_TEST_FLAGS_NORACE=("${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}")
  GO_TEST_FLAGS+=("-coverprofile=${COVERAGE_FILE}")
  GO_TEST_FLAGS_NORACE+=("-coverprofile=${COVERAGE_FILE_NORACE}")
else
  GO_TEST_FLAGS_NORACE=("${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}")
fi

# Run api unit tests
# Since api is a different go module
go test "${API_TEST_FLAGS[@]+"${API_TEST_FLAGS[@]}"}" \
    github.com/vmware-tanzu/vm-operator/api/...

# Run unit tests
# go test: -race requires cgo
# shellcheck disable=SC2046
CGO_ENABLED=1 \
go test "${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}" \
  $(join_packages_for_tests "${TEST_PKGS[@]}") \
  "${GINKGO_FLAGS[@]+"${GINKGO_FLAGS[@]}"}" \
  -- \
  -enable-unit-tests=true \
  -enable-integration-tests=false

# Run certain unit tests without -race
# shellcheck disable=SC2046
go test "${GO_TEST_FLAGS_NORACE[@]+"${GO_TEST_FLAGS_NORACE[@]}"}" \
  $(join_packages_for_tests "${TEST_PKGS_NORACE[@]}") \
  "${GINKGO_FLAGS[@]+"${GINKGO_FLAGS[@]}"}" \
  -- \
  -enable-unit-tests=true \
  -enable-integration-tests=false

# Merge the race/norace code coverage files.
if [ -n "${COVERAGE_FILE:-}" ]; then
  TMP_COVERAGE_FILE="$(mktemp)"
  mv "${COVERAGE_FILE}" "${TMP_COVERAGE_FILE}"
  gocovmerge "${API_COVERAGE_FILE}" "${TMP_COVERAGE_FILE}" "${COVERAGE_FILE_NORACE}" >"${COVERAGE_FILE}"
  rm -f "${API_COVERAGE_FILE}" "${TMP_COVERAGE_FILE}" "${COVERAGE_FILE_NORACE}"
fi
