#!/usr/bin/env bash

# Install the Go tools needed for development and testing.

set -o errexit
set -o pipefail
set -o nounset

goInstall() {
    command -v "$1" &> /dev/null || go get -u "$2"
}

goInstall "dep"             "github.com/golang/dep/cmd/dep"
goInstall "ginkgo"          "github.com/onsi/ginkgo/ginkgo"
goInstall "go-junit-report" "github.com/jstemmer/go-junit-report"
goInstall "golangci-lint"   "github.com/golangci/golangci-lint/cmd/golangci-lint"
goInstall "vcsim"           "github.com/vmware/govmomi/vcsim"

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
