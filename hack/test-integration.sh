#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

go clean -testcache > /dev/null

# The multiple TestMain() suites run in parallel (but the tests within a package are
# sequential). This ends up causing port conflicts so run each test separately.
#go test -parallel=1 -tags=integration -v ./cmd/... ./pkg/...
go test -parallel=1 -tags=integration -v ./pkg/controller/virtualmachine/...
go test -parallel=1 -tags=integration -v ./pkg/controller/virtualmachineclass/...
go test -parallel=1 -tags=integration -v ./pkg/controller/virtualmachineservice/...

# Upstream support for these tests were removed.
#ginkgo -v ./test/integration/...
