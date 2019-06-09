#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

isInstalled() {
    command -v "$1" &> /dev/null
}

installApps() {
    isInstalled "git"       || brew install git
    isInstalled "kubectl"   || brew install kubernetes-cli

    isInstalled "docker"    || brew cask install docker
    isInstalled "minikube"  || brew cask install minikube

    #
    # executables required to run load-k8s-master for WCP testbeds
    #

    # Bash version 4 or higher is required for load-k8s-master
    isInstalled "/usr/local/bin/bash"          || brew install bash
    isInstalled "md5sum"                       || brew install md5sum
    # macOS uses BSD getopt. load-k8s-master assumes GNU getopt.
    isInstalled "/usr/local/opt/gnu-getop/bin" || brew install gnu-getopt
    isInstalled "sshpass"                      || brew install http://git.io/sshpass.rb
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
