#!/usr/bin/env bash
  
set -o errexit
set -o pipefail
set -o nounset
set -x

if docker 2>/dev/null; then
	docker run --rm -t -v "$(pwd)":/build:ro gcr.io/cluster-api-provider-vsphere/extra/shellcheck
else
	if shellcheck -V >/dev/null; then
		find . -path ./vendor -prune -o -name "*.*sh" -type f -print0 | xargs -0 shellcheck "${@}"
	else
		echo "No shellcheck linter found"
		exit 1
	fi
fi
