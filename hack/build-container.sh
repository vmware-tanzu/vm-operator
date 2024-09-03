#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x


CRI_BIN="${CRI_BIN:-}"
if [ -z "${CRI_BIN:-}" ]; then
  CRI_BIN="$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)"
fi

IMAGE=
IMAGE_TAG=
BUILD_NUMBER=

usage() {
    echo "Usage: $(basename "${0}") -i image -t imageTag [-n buildNumber] [-c commit] [-b branch] [-v version]"
    exit 1
}

build() {
    GOOS=linux
    echo "GOOS=${GOOS} GOARCH=${GOARCH}"
    "${CRI_BIN}" build . \
        -t "${IMAGE}":"${IMAGE_TAG}" \
        -t "${IMAGE}":"${BUILD_NUMBER}" \
        -t "${IMAGE}":"${BUILD_VERSION}" \
        --build-arg BUILD_BRANCH="${BUILD_BRANCH}" \
        --build-arg BUILD_COMMIT="${BUILD_COMMIT}" \
        --build-arg BUILD_NUMBER="${BUILD_NUMBER}" \
        --build-arg BUILD_VERSION="${BUILD_VERSION}" \
        --build-arg TARGETOS="${GOOS}" \
        --build-arg TARGETARCH="${GOARCH}"
}

while getopts ":i:t:n:c:b:v:" opt ; do
    case $opt in
        "i" ) IMAGE="${OPTARG}" ;;
        "t" ) IMAGE_TAG="${OPTARG}" ;;
        "b" ) BUILD_BRANCH="${OPTARG}" ;;
        "c" ) BUILD_COMMIT="${OPTARG}" ;;
        "n" ) BUILD_NUMBER="${OPTARG}" ;;
        "v" ) BUILD_VERSION="${OPTARG}" ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

if [[ -z "${IMAGE:-}" || -z "${IMAGE_TAG:-}" ]] ; then
    usage
fi

BUILD_NUMBER="${BUILD_NUMBER:-0}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse HEAD)}"
BUILD_BRANCH="${BUILD_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"

build
