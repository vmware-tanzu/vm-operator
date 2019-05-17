#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

isInstalled() {
    command -v "$1" &> /dev/null
}

installApps() {
    isInstalled "git" 	    || brew install git
    isInstalled "kubectl"   || brew install kubernetes-cli

    isInstalled "docker"    || brew cask install docker
    isInstalled "minikube"  || brew cask install minikube
}

installGo() {
    isInstalled "go" || brew install golang

    hack/install-go-tools.sh
}

installKubebuilder() {
    if [[ ! -d /usr/local/kubebuilder ]] ; then
        hack/install-kubebuilder.sh
    fi
}

if ! isInstalled "brew" ; then
    echo "Install homebrew. See https://docs.brew.sh/Installation"
    exit 1
fi

installApps
installGo
installKubebuilder

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
