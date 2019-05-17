#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

VERSION=1.0.8 # latest stable version
OS=$(go env GOOS)
ARCH=$(go env GOARCH)

curl -L -O "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${VERSION}/kubebuilder_${VERSION}_${OS}_${ARCH}.tar.gz"

# No need for sudo in Jenkins.
[[ -z ${WORKSPACE:-} ]] && SUDO=sudo || SUDO=""

TAR=kubebuilder_${VERSION}_${OS}_${ARCH}.tar.gz
tar -zxvf $TAR
mv kubebuilder_${VERSION}_${OS}_${ARCH} kubebuilder
rm $TAR
$SUDO mv kubebuilder /usr/local

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
