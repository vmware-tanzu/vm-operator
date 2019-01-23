#!/usr/bin/env bash

set -o errexit
set -o nounset

isInstalled() {
    command -v "$1" &> /dev/null
}

installApps() {
    isInstalled "git" 	    || brew install git
    isInstalled "hg" 	    || brew install mercurial
    isInstalled "kubectl"   || brew install kubernetes-cli

    isInstalled "docker"    || brew cask install docker
    isInstalled "minikube"  || brew cask install minikube
}

installGo() {
    isInstalled "go" || brew install golang

    isInstalled "dep"    || go get -u github.com/golang/dep/cmd/dep
    isInstalled "golint" || go get -u github.com/golang/lint/golint
    isInstalled "ginkgo" || go get -u github.com/onsi/ginkgo/ginkgo
}

if ! isInstalled "brew" ; then
    echo "Install homebrew. See https://docs.brew.sh/Installation"
    exit 1
fi

installApps
installGo
