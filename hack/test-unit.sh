#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

usage () {
    echo "Usage: $(basename $0) [-c coverage]"
    exit 1
}

OPTIONS=""

while getopts ":c:" opt ; do
    case $opt in
        "c" ) OPTIONS="$OPTIONS -coverprofile=$OPTARG -coverpkg=./pkg/...,./cmd/..." ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

go clean -testcache > /dev/null

go test -race -v $OPTIONS ./cmd/... ./pkg/...

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
