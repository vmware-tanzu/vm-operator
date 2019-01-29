#!/usr/bin/env bash

set -e
set -x

COMMIT=09ac1e0d568438dbf3f0825b617eea516b5e02a6
VERSION=1.12
TARGETS=darwin:amd64

#go get -d github.com/kubernetes-incubator/apiserver-builder/...
cd "$(go env GOPATH)/src/github.com/kubernetes-incubator/apiserver-builder"

go run ./cmd/apiserver-builder-release/main.go vendor --version ${VERSION}.alpha.0 --commit $COMMIT --kubernetesVersion $VERSION
go run ./cmd/apiserver-builder-release/main.go build --version ${VERSION}.alpha.0 --targets $TARGETS
