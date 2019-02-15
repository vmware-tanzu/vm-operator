#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

go test -v -parallel 4 ./test/integration/...
