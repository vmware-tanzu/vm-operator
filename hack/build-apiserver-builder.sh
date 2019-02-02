#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

COMMIT=09ac1e0d568438dbf3f0825b617eea516b5e02a6
DEFAULT_K8S_VERSION="1.12"

# No need for cross compile.
GOOS=$(uname -s | awk '{print tolower($0)}')
GOARCH=amd64

usage() {
    echo "Usage: $(basename $0) [-k k8sVersion]"
    exit 1
}

checkout() {
    # go get -d github.com/kubernetes-incubator/apiserver-builder-alpha/... complains about
    # missing packages.
    [[ -d ".git" ]] ||
        git clone https://github.com/kubernetes-incubator/apiserver-builder-alpha.git .
}

build() {
    # HEAD is busted. Use working commit.
    #git checkout -b build 7c92455d9530fa6a4c65bf32e943976d8022a459

    go run ./cmd/apiserver-builder-release/main.go vendor --version $VERSION --commit $COMMIT --kubernetesVersion $K8S_VERSION
    go run ./cmd/apiserver-builder-release/main.go build --version $VERSION --targets $GOOS:$GOARCH
}

install() {
    # This does not accept a --targets argument.
    GOOS=$GOOS GOARCH=$GOARCH go run ./cmd/apiserver-builder-release/main.go install --version $VERSION
}

while getopts ":k:" opt ; do
    case $opt in
        "k" ) k8sVersion=$OPTARG ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

K8S_VERSION=${k8sVersion:-$DEFAULT_K8S_VERSION}
VERSION=${K8S_VERSION}.alpha.0

DIR="$(go env GOPATH)/src/github.com/kubernetes-incubator/apiserver-builder-alpha"
mkdir -p "$DIR"
cd "$DIR"

checkout
build
install

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
