#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

IMAGE=
IMAGE_TAG=
BUILD_NUMBER=
COMMIT=
BRANCH=
VERSION="0.0"

usage() {
    echo "Usage: $(basename "$0") -i image -t imageTag [-n buildNumber] [-c commit] [-b branch] [-v version]"
    exit 1
}

build() {
    GOOS=linux
    echo "GOOS=${GOOS} GOARCH=${GOARCH}"
    docker build . \
        -t $IMAGE:$IMAGE_TAG \
        -t $IMAGE:$BUILD_NUMBER \
        -t $IMAGE:$VERSION \
        --build-arg buildNumber=${BUILD_NUMBER} \
        --build-arg commit=${COMMIT} \
        --build-arg branch=${BRANCH} \
        --build-arg version=${VERSION} \
        --build-arg TARGETOS=${GOOS} \
        --build-arg TARGETARCH=${GOARCH}
}

while getopts ":i:t:n:c:b:v:" opt ; do
    case $opt in
        "i" ) IMAGE=$OPTARG ;;
        "t" ) IMAGE_TAG=$OPTARG ;;
        "n" ) BUILD_NUMBER=$OPTARG ;;
        "c" ) COMMIT=$OPTARG ;;
        "b" ) BRANCH=$OPTARG ;;
        "v" ) VERSION=$OPTARG ;;
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

build
