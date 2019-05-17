#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Default 1m0s sometimes timeouts in Jenkins.
golangci-lint run --fast --deadline 2m0s
