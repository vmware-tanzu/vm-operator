#!/usr/bin/env bash
  
set -o errexit
set -o pipefail
set -o nounset
set -x

if docker 2>/dev/null; then
	docker run --rm -v "$(pwd)":/build gcr.io/cluster-api-provider-vsphere/extra/mdlint:0.17.0
else
	if markdownlint >/dev/null; then
		markdownlint -i vendor .
	else
		echo "No markdown linter found"
		exit 1
	fi
fi
