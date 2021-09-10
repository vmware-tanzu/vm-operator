#!/usr/bin/env bash
# Check if generated files are up to date
# Used in the precommit Jenkins job

set -o errexit
set -o nounset
set -o pipefail

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

make generate

git_status=$(git status config hack pkg --porcelain)
if [ -z "${git_status}" ]; then
    echo "Generated files are up to date!"
else
    echo "Error: You may haven't check in your generated files!" \
        "You can refer to this thread to see why we need to check in generated files." \
        "Following files are modified after re-generating" \
        "${git_status}" 1>&2
    exit 1
fi
