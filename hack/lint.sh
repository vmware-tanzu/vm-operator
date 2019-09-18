#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

# Run those linters if run locally
# See 
if [[ -z "${OPTIONS}" ]]; then
  OPTIONS="-E vet -E vetshadow -E staticcheck"
fi
golangci-lint run --verbose ${OPTIONS}
