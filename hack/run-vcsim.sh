#!/usr/bin/env bash

ADDRESS=$1

set -o errexit
set -o pipefail
set -o nounset

# Just grab from govmomi HEAD until we have a reason to differ.
# At the moment, just run local.  Eventually may need to move to a container.
go get -u github.com/vmware/govmomi/vcsim
vcsim -l $ADDRESS

