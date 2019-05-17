#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

COMMIT=a1abecd3833a0188bab06e41aeca0559115b3007
INSTALL=1
DEFAULT_K8S_VERSION="1.12"

# No need for cross compile.
GOOS=$(uname -s | awk '{print tolower($0)}')
GOARCH=amd64

usage() {
    echo "\
Usage: $(basename $0) [-I] [-k k8sVersion]
    -I: Do not install to \$GOPATH
"
    exit 1
}

checkout() {
    url=git@gitlab.eng.vmware.com:core-build/mirrors_github_apiserver-builder-alpha.git
    if [[ ! -d ".git" ]] ; then
        git clone $url .
    else
        git remote set-url origin $url
        git fetch origin
    fi
    git checkout $COMMIT
}

build() {
    [[ -d "release/$VERSION" ]] && rm -r "release/$VERSION"

    go run ./cmd/apiserver-builder-release/main.go vendor --version $VERSION --commit $COMMIT --kubernetesVersion $K8S_VERSION
    go run ./cmd/apiserver-builder-release/main.go build --version $VERSION --targets $GOOS:$GOARCH
}

install() {
    # This does not accept a --targets argument.
    GOOS=$GOOS GOARCH=$GOARCH go run ./cmd/apiserver-builder-release/main.go install --version $VERSION
}

while getopts ":k:I" opt ; do
    case $opt in
        "k" ) k8sVersion=$OPTARG ;;
        "I" ) INSTALL= ;;
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

if [[ -n $INSTALL ]] ; then
    install
else
    echo "Add $DIR/release/$VERSION/bin to your PATH"
fi

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
