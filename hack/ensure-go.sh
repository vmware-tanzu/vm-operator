#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# MIN_GO_VERSION is the minimum, supported Go version.
MIN_GO_VERSION="go${MIN_GO_VERSION:-1.12.1}"

# Ensure the go tool exists and is a viable version.
verify_go_version() {
  if ! command -v go >/dev/null 2>&1; then
    cat <<EOF
Can't find 'go' in PATH, please fix and retry.
See http://golang.org/doc/install for installation instructions.
EOF
    return 2
  fi

  local go_version
  IFS=" " read -ra go_version <<<"$(go version)"
  if [ "${go_version[2]}" != 'devel' ] && \
     [ "${MIN_GO_VERSION}" != "$(printf "%s\\n%s" "${MIN_GO_VERSION}" "${go_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1)" ]; then
    cat <<EOF
Detected go version: ${go_version[*]}.
Kubernetes requires ${MIN_GO_VERSION} or greater.
Please install ${MIN_GO_VERSION} or later.
EOF
    return 2
  fi
}

verify_go_version

# Explicitly opt into go modules.
export GO111MODULE=on

# TODO(akutz) Cannot use GOPROXY until VMware sets up its own mirror that
#             is aware of gitlab.eng.vmware.com.
export GOPROXY=""
