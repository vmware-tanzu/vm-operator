#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

IMAGE=
IMAGE_TAG=
BUILD_NUMBER=
BUILD_TAG=
COMMIT=
BRANCH=
BINDIR=

usage() {
    echo "Usage: $(basename $0) -i image -t imageTag [-n buildNumber] [-T buildTag] [-c commit] [-b branch] [-B binDir]"
    exit 1
}

# We do not use the 'apiserver-boot build container ...' for a few reasons:
#   - the Dockerfile used is hardcoded into the source and the base image is quite old
#   - it does not work well building the container in the Jenkins pipeline
build() {
    docker build -t $IMAGE:$IMAGE_TAG -f Dockerfile.build \
        --build-arg buildNumber=${BUILD_NUMBER} \
        --build-arg buildTag=${BUILD_TAG} \
        --build-arg commit=${COMMIT} \
        --build-arg branch=${BRANCH} \
        --rm \
        $BINDIR
}

while getopts ":i:t:n:T:c:b:B:" opt ; do
    case $opt in
        "i" ) IMAGE=$OPTARG ;;
        "t" ) IMAGE_TAG=$OPTARG ;;
        "n" ) BUILD_NUMBER=$OPTARG ;;
        "T" ) BUILD_TAG=$OPTARG ;;
        "c" ) COMMIT=$OPTARG ;;
        "b" ) BRANCH=$OPTARG ;;
        "B" ) BINDIR=$OPTARG ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

if [[ -z ${IMAGE:-} || -z ${IMAGE_TAG:-} ]] ; then
    usage
fi

BUILD_NUMBER=${BUILD_NUMBER:-0}
COMMIT=${COMMIT:-$(git rev-parse HEAD)}
BRANCH=${BRANCH:-$(git rev-parse --abbrev-ref HEAD)}
BINDIR=${BINDIR:-bin/}

build

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
