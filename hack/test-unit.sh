#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

go clean -testcache > /dev/null

go test -v ./cmd/... ./pkg/...
