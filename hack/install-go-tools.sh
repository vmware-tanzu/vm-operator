#!/usr/bin/env bash

# Install the Go tools needed for development and testing.

set -o errexit
set -o pipefail
set -o nounset

goInstall() {
    command -v "$1" &> /dev/null || go get -u "$2"
}

goInstall "dep"               "github.com/golang/dep/cmd/dep"
goInstall "vcsim"             "github.com/vmware/govmomi/vcsim"
goInstall "ginkgo"            "github.com/onsi/ginkgo/ginkgo"
goInstall "goimports"         "golang.org/x/tools/cmd/goimports"
goInstall "golangci-lint"     "github.com/golangci/golangci-lint/cmd/golangci-lint"
goInstall "go-junit-report"   "github.com/jstemmer/go-junit-report"
goInstall "gocover-cobertura" "github.com/t-yuki/gocover-cobertura"
goInstall "gocovmerge"        "github.com/wadey/gocovmerge"

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
