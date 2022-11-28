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

COVER_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
COVER_OPTS=$(join_packages_for_cover "${COVER_PKGS[@]}")

# Packages tested with the new test framework.
TEST_PKGS=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)

GINKGO_FLAGS=()
GO_TEST_FLAGS=("-v" "-race")

if [ -n "${JOB_NAME:-}" ]; then
  # Disable color output on Jenkins.
  GINKGO_FLAGS+=("-ginkgo.noColor")
fi

# GitHub actions store the test cache, so do not force tests
# to re-run when running as part of a GitHub action.
if [ -z "${GITHUB_ACTION:-}" ]; then
  GO_TEST_FLAGS+=("-count=1")
fi

# The first argument is the name of the coverage file to use.
if [ -n "${COVERAGE_FILE}" ]; then
  GO_TEST_FLAGS+=("-coverprofile=${COVERAGE_FILE}" "-coverpkg=${COVER_OPTS}")
fi

# Run integration tests
# go test: -race requires cgo
# shellcheck disable=SC2046
CGO_ENABLED=1 \
go test "${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}" \
  $(join_packages_for_tests "${TEST_PKGS[@]}") \
  "${GINKGO_FLAGS[@]+"${GINKGO_FLAGS[@]}"}" \
  -- \
  -enable-unit-tests=false \
  -enable-integration-tests=true || \
  TEST_CMD_EXIT_CODE="${?}"

# TEST_CMD_EXIT_CODE may be set to 2 if there are any tests marked as
# pending/skipped. This pattern is used by developers to leave test
# code in place that is flaky or needs some attention, but does not
# warrant total removal. Therefore if the TEST_CMD_EXIT_CODE is 2, it
# still counts as a successful exit code.
{ [ "${TEST_CMD_EXIT_CODE:-0}" -eq "2" ] && TEST_CMD_EXIT_CODE=0; } || true

# Just in case the test report did not catch a test failure, ensure
# this program exits with the TEST_CMD_EXIT_CODE if it is non-zero.
exit "${TEST_CMD_EXIT_CODE:-0}"
