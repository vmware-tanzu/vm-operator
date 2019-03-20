#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Clearing the cache to ensure that the tests
# are actually run each time.
go clean -testcache

go test -v -parallel 4 ./test/integration/...
