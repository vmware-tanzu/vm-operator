#!/usr/bin/env bash

# Run the tests.
# If an argument is given, the coverage will be recorded:
# test.sh [<coverage file>]

set -o errexit
set -o nounset
set -o pipefail
set -x

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

COVERAGE_FILE="${1:-}"

TEST_PKGS="${TEST_PKGS:-./api ./controllers ./pkg ./webhooks}"

GO_TEST_FLAGS=("-v" "-r" "--race" "--keep-going")

# The first argument is the name of the coverage file to use.
if [ -n "${COVERAGE_FILE}" ]; then
  GO_TEST_FLAGS+=("--cover")
  GO_TEST_FLAGS+=("--covermode=atomic")
  GO_TEST_FLAGS+=("--coverprofile=${COVERAGE_FILE}")
fi

# Run the tests.
# shellcheck disable=SC2086
ginkgo "${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}" \
  ${TEST_PKGS} || \
  TEST_CMD_EXIT_CODE="${?}"

# TEST_CMD_EXIT_CODE may be set to 2 if there are any tests marked as
# pending/skipped. This pattern is used by developers to leave test
# code in place that is flaky or needs some attention, but does not
# warrant total removal. Therefore if the TEST_CMD_EXIT_CODE is 2, it
# still counts as a successful exit code.
if [ "${TEST_CMD_EXIT_CODE:-0}" -eq "2" ]; then
  TEST_CMD_EXIT_CODE=0
fi

# Just in case the test report did not catch a test failure, ensure
# this program exits with the TEST_CMD_EXIT_CODE if it is non-zero.
exit "${TEST_CMD_EXIT_CODE:-0}"
