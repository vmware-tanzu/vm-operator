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

if [ -z "${GOHOSTOS:-}" ] || [ -z "${GOHOSTARCH:-}" ]; then
    if ! command -v go >/dev/null 2>&1; then
        echo "GOHOSTOS and/or GOHOSTARCH are unset and golang is not detected" 1>&2
        exit 1
    fi
fi

GOHOSTOS="${GOHOSTOS:-$(go env GOHOSTOS)}"
GOHOSTARCH="${GOHOSTARCH:-$(go env GOHOSTARCH)}"

GOOS=linux
GOARCH="${GOARCH:-${GOHOSTARCH}}"

usage() {
    echo "Usage: $(basename "${0}") -i image -t imageTag [-n buildNumber] [-c commit] [-b branch] [-v version] [-o outFile]"
    exit 1
}

build_docker() {
    if [ "${GOOS}" = "${GOHOSTOS}" ] && [ "${GOARCH}" = "${GOHOSTARCH}" ]; then

        "${CRI_BIN}" build . \
            -t "${IMAGE}":"${IMAGE_TAG}" \
            -t "${IMAGE}":"${BUILD_NUMBER}" \
            -t "${IMAGE}":"${BUILD_VERSION}" \
            --build-arg BUILD_BRANCH="${BUILD_BRANCH}" \
            --build-arg BUILD_COMMIT="${BUILD_COMMIT}" \
            --build-arg BUILD_NUMBER="${BUILD_NUMBER}" \
            --build-arg BUILD_VERSION="${BUILD_VERSION}"

    elif docker buildx version >/dev/null 2>&1; then

        "${CRI_BIN}" buildx build . \
            -t "${IMAGE}":"${IMAGE_TAG}" \
            -t "${IMAGE}":"${BUILD_NUMBER}" \
            -t "${IMAGE}":"${BUILD_VERSION}" \
            --platform "${GOOS}/${GOARCH}" \
            --build-arg BUILD_BRANCH="${BUILD_BRANCH}" \
            --build-arg BUILD_COMMIT="${BUILD_COMMIT}" \
            --build-arg BUILD_NUMBER="${BUILD_NUMBER}" \
            --build-arg BUILD_VERSION="${BUILD_VERSION}"

    else

        echo "docker buildx is not installed" 1>&2
        exit 1

    fi
}

build_podman() {
    "${CRI_BIN}" build . \
        -t "${IMAGE}":"${IMAGE_TAG}" \
        -t "${IMAGE}":"${BUILD_NUMBER}" \
        -t "${IMAGE}":"${BUILD_VERSION}" \
        --os "${GOOS}" --arch "${GOARCH}" \
        --build-arg BUILD_BRANCH="${BUILD_BRANCH}" \
        --build-arg BUILD_COMMIT="${BUILD_COMMIT}" \
        --build-arg BUILD_NUMBER="${BUILD_NUMBER}" \
        --build-arg BUILD_VERSION="${BUILD_VERSION}"
}

build() {
    echo "GOOS=${GOOS} GOARCH=${GOARCH}"

    if [[ "${CRI_BIN}" == *"podman"* ]]; then
        build_podman
    elif [[ "${CRI_BIN}" == *"docker"* ]]; then
        build_docker
    else
        echo "unsupported cri: '${CRI_BIN}'" 1>&2
        exit 1
    fi

    if [ -n "${OUT_FILE:-}" ]; then
        mkdir -p "$(dirname "${OUT_FILE}")"
        "${CRI_BIN}" save "${IMAGE}":"${IMAGE_TAG}" -o "${OUT_FILE}"
    fi
}

while getopts ":i:t:n:c:b:v:o:" opt ; do
    case $opt in
        "i" ) IMAGE="${OPTARG}" ;;
        "t" ) IMAGE_TAG="${OPTARG}" ;;
        "b" ) BUILD_BRANCH="${OPTARG}" ;;
        "c" ) BUILD_COMMIT="${OPTARG}" ;;
        "n" ) BUILD_NUMBER="${OPTARG}" ;;
        "v" ) BUILD_VERSION="${OPTARG}" ;;
        "o" ) OUT_FILE="${OPTARG}" ;;
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
