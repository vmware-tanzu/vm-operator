#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

golangci-lint run --verbose
