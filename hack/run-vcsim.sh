#!/usr/bin/env bash

ADDRESS=$1

set -o errexit
set -o pipefail
set -o nounset

# At the moment, just run local.  Eventually may need to move to a container.
vcsim -l $ADDRESS

