#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x


################################################################################
##                                     functions
################################################################################


usage() {
    echo "Usage: $(basename "${0}") -i image -t imageTag -v version [-n buildNumber] [-c commit] [-b branch] [-V imageVersion] [-o outFile] [-B baseImage] [-C cri]"
    exit 1
}

init_docker_build_flags() {
    if [ "${GOARCH}" = "${GOHOSTARCH}" ]; then
        return 0
    fi

    if docker buildx version >/dev/null 2>&1; then
        BUILD_SUB_COMMAND="buildx build"
        BUILD_FLAGS+=(--platform "${GOOS}/${GOARCH}")
    else
        echo "docker buildx is not installed" 1>&2
        exit 1
    fi
}

init_podman_build_flags() {
    BUILD_FLAGS+=(--os "${GOOS}")
    BUILD_FLAGS+=(--arch "${GOARCH}")
}

init_build_flags() {
    BUILD_FLAGS+=(-t "${IMAGE}":"${IMAGE_TAG}")
    BUILD_FLAGS+=(-t "${IMAGE}":"${BUILD_NUMBER}")
    BUILD_FLAGS+=(-t "${IMAGE}":"${IMAGE_VERSION}")
    BUILD_FLAGS+=(--build-arg BUILD_BRANCH="${BUILD_BRANCH}")
    BUILD_FLAGS+=(--build-arg BUILD_COMMIT="${BUILD_COMMIT}")
    BUILD_FLAGS+=(--build-arg BUILD_NUMBER="${BUILD_NUMBER}")
    BUILD_FLAGS+=(--build-arg BUILD_VERSION="${BUILD_VERSION}")

    if [ -n "${BASE_IMAGE:-}" ]; then
        BUILD_FLAGS+=(--build-arg BASE_IMAGE="${BASE_IMAGE}")
    fi

    if [[ "${CRI_BIN}" == *"podman"* ]]; then
        init_podman_build_flags
    elif [[ "${CRI_BIN}" == *"docker"* ]]; then
        init_docker_build_flags
    else
        echo "unsupported cri: '${CRI_BIN}'" 1>&2
        exit 1
    fi

    if [ -n "${ADDITIONAL_CRI_BUILD_FLAGS:-}" ]; then
        BUILD_FLAGS+=(${ADDITIONAL_CRI_BUILD_FLAGS:-})
    fi
}

build() {
    echo "GOOS=${GOOS} GOARCH=${GOARCH}"

    init_build_flags

    "${CRI_BIN}" ${BUILD_SUB_COMMAND:-build} . "${BUILD_FLAGS[@]+"${BUILD_FLAGS[@]}"}"

    if [ -n "${OUT_FILE:-}" ]; then
        mkdir -p "$(dirname "${OUT_FILE}")"
        "${CRI_BIN}" save "${IMAGE}":"${IMAGE_VERSION}" -o "${OUT_FILE}"
    fi
}


################################################################################
##                                      main
################################################################################

if [ -z "${GOHOSTARCH:-}" ]; then
    if ! command -v go >/dev/null 2>&1; then
        echo "GOHOSTARCH is unset and golang is not detected" 1>&2
        exit 1
    fi
    GOHOSTARCH="$(go env GOHOSTARCH)"
fi

GOOS=linux
GOARCH="${GOARCH:-${GOHOSTARCH}}"

while getopts ":i:t:n:c:b:v:V:o:B:C:" opt ; do
    case $opt in
        "i" ) IMAGE="${OPTARG}" ;;
        "t" ) IMAGE_TAG="${OPTARG}" ;;
        "b" ) BUILD_BRANCH="${OPTARG}" ;;
        "c" ) BUILD_COMMIT="${OPTARG}" ;;
        "n" ) BUILD_NUMBER="${OPTARG}" ;;
        "v" ) BUILD_VERSION="${OPTARG}" ;;
        "V" ) IMAGE_VERSION="${OPTARG}" ;;
        "o" ) OUT_FILE="${OPTARG}" ;;
        "B" ) BASE_IMAGE="${OPTARG}" ;;
        "C" ) CRI_BIN="${OPTARG}" ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

if [[ -z "${IMAGE:-}" || -z "${IMAGE_TAG:-}" || -z "${BUILD_VERSION}" ]] ; then
    usage
fi

if [ -z "${CRI_BIN:-}" ]; then
    CRI_BIN="$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)"
fi

BUILD_NUMBER="${BUILD_NUMBER:-0}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse HEAD)}"
BUILD_BRANCH="${BUILD_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"
IMAGE_VERSION="${IMAGE_VERSION:-${BUILD_VERSION//+/-}}"

build
