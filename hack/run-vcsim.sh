#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

usage () {
    echo "Usage: $(basename $0) <listen address>"
    exit 1
}

while getopts ":" opt ; do
    case $opt in
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 1 ]] ; then
    usage
fi

ADDRESS=$1

# At the moment, just run local.  Eventually may need to move to a container.
exec vcsim -l $ADDRESS

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
