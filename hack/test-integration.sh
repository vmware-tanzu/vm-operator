#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Use tag to distinguish unit and integration tests.
GOTEST="go test -tags=integration -v"

RC=0

usage () {
    echo "Usage: $(basename $0) [-c coverage]"
    exit 1
}

test_controller() {
    PKG=${1%/}
    CMD="$GOTEST -parallel=1"

    if [[ -n $COVERAGE ]] ; then
        OUT=${PKG//\//-}.out
        COVERAGE_FILES+=($OUT)
        CMD="$CMD -coverprofile=$OUT -coverpkg=./pkg/..."
    fi

    if ! $CMD ./${PKG}/... ; then
        RC=1
    fi
}

COVERAGE=""
COVERAGE_FILES=()

while getopts ":c:" opt ; do
    case $opt in
        "c" ) COVERAGE=$OPTARG ;;
        \? ) usage ;;
    esac
done

shift $((OPTIND - 1))

if [[ $# -ne 0 ]] ; then
    usage
fi

go clean -testcache > /dev/null

# Cannot run all integration tests the easy way:
#    $ go test -parallel=1 -tags=integration -v ./cmd/... ./pkg/...
#
# because each controller suite has a TestMain() and parallel seems to
# only apply to the tests run by each TestMain(). Since the aggregated
# apiserver has to listen on port 443 this causes a port conflict. So
# run the tests separately and merge the coverage output.

for CTRL in pkg/controller/*/ ; do
    test_controller $CTRL
done

if [[ ${#COVERAGE_FILES[@]} -gt 0 ]] ; then
    gocovmerge ${COVERAGE_FILES[@]} > $COVERAGE
    rm ${COVERAGE_FILES[@]}
fi

# Upstream support for these tests was removed.
#ginkgo -v ./test/integration/...

exit $RC

# vim: tabstop=4 shiftwidth=4 expandtab softtabstop=4 filetype=sh
