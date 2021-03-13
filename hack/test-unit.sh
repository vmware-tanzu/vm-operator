#!/usr/bin/env bash

# Run the unit tests.
# If an argument is given, the coverage will be recorded:
# test-unit.sh [<coverage file>]

set -o errexit
set -o nounset
set -o pipefail

function join_packages { local IFS=","; echo "$*"; }

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# shellcheck disable=SC1091
source hack/ensure-go.sh

# Initialize GOFLAGS by indicating verbose output.
GOFLAGS="-v"

# The "-count=1" argument is the idiomatic way to ensure all tests
# are run and the cached test results are ignored. Please see
# the output of "go help test" for more information.
GOFLAGS="${GOFLAGS} -count=1"

# The first argument is the name of the coverage file to use.
coverage_file="${1-}"
packages=(
  "./controllers/..."
  "./pkg/..."
  "./webhooks/..."
)
cov_opts=$(join_packages "${packages[@]}")

if [[ -n ${coverage_file} ]]; then
    GOFLAGS="${GOFLAGS} -coverprofile=${coverage_file} -coverpkg=${cov_opts} -covermode=atomic"
fi

# Tell Go how to do things.
export GOFLAGS

# Execute the tests.
go test ./controllers/... ./pkg/... ./webhooks/...
