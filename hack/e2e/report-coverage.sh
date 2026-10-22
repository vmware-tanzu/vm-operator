#!/usr/bin/env bash
# © Broadcom. All Rights Reserved.
# The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./callback.sh
source "${SCRIPT_DIR}/callback.sh"

coverage_exit=0
python3 "${SCRIPT_DIR}/coverage.py" \
    --output "${E2E_ARTIFACT_FOLDER:?}/coverage" aggregate || coverage_exit=$?
if [ "${coverage_exit}" -eq 0 ]; then
    report_callback "Passed"
else
    report_callback "Failed" "Combined coverage is incomplete; see coverage/status.json and available reports"
fi
exit "${coverage_exit}"
