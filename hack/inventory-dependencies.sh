#!/usr/bin/env bash

# Inventory the open source dependencies used by the project.

set -o errexit
set -o pipefail
set -o nounset
set -x

BINARY_NAME=${1?Must supply the name of the binary}
PACKAGE_NAME=${2?Must supply the name of the package}

CURDIR="$(pwd)"
TMPDIR="$(mktemp -d)"
trap '{ rm -rf "$TMPDIR"; }' EXIT

cd "${TMPDIR}" || exit

# We use osstptool to inventory our open source dependencies
git clone --single-branch --branch vmware-master --recurse-submodules --depth 1 git@gitlab.eng.vmware.com:core-build/cayman_osstptool
cd cayman_osstptool/osstptool/src && make osstptool

cd "${TMPDIR}" || exit

./cayman_osstptool/osstptool/src/osstptool generate --root="${CURDIR}" "${BINARY_NAME}" "${PACKAGE_NAME}" -e "${BINARY_NAME}"

cp "${TMPDIR}/osstp_golang.yml" "${CURDIR}"
