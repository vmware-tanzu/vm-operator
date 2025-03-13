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

GO_TEST_FLAGS+=("-v")           # verbose
GO_TEST_FLAGS+=("-r")           # recursive
GO_TEST_FLAGS+=("--race")       # check for possible races
GO_TEST_FLAGS+=("--keep-going") # do not fail on the first error

# disable govet's printf analyzer during tests until the issue
# outlined at https://github.com/golang/go/issues/60529#issuecomment-2043669945
# and https://github.com/kubernetes/kubernetes/issues/127191
# is able to be solved at point-of-use.
#
# since ginkgo does not allow disabling a specific analyzer, the list
# must contain all *but* the printf analyzer
GO_TEST_FLAGS+=("--vet=appends,asmdecl,assign,atomic,bools,buildtag,cgocall,composites,copylocks,defers,directive,errorsas,framepointer,httpresponse,ifaceassert,loopclosure,lostcancel,nilfunc,shift,sigchanyzer,slog,stdmethods,stringintconv,structtag,testinggoroutine,tests,timeformat,unmarshal,unreachable,unsafeptr,unusedresult")

# Only run tests that match given labels if LABEL_FILTER is non-empty.
if [ -n "${LABEL_FILTER:-}" ]; then
  GO_TEST_FLAGS+=("--label-filter" "${LABEL_FILTER:-}")
fi

# Coverage is always enabled, it is just not always recorded to an output file.
GO_TEST_FLAGS+=("--cover")
GO_TEST_FLAGS+=("--covermode=atomic")

# Record coverage to an output file if COVERAGE_FILE is non-empty.
if [ -n "${COVERAGE_FILE:-}" ]; then
  GO_TEST_FLAGS+=("--coverprofile=${COVERAGE_FILE:-}")
fi

# Run the tests.
# shellcheck disable=SC2086
ginkgo "${GO_TEST_FLAGS[@]+"${GO_TEST_FLAGS[@]}"}" "${@:-}" || TEST_CMD_EXIT_CODE="${?}"

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
