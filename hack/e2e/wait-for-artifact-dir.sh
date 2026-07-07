#!/bin/bash
set -ex

#
# wait-for-artifact-dir.sh - Wait for RESULTSDIR to exist and make it writable
#
# The UTS NFS log directory is created by the svc-vdm_stage0 service account.
# This script waits for it to appear then uses "sudo -u svc-vdm_stage0 chmod g+w"
# to make it group-writable. Both vmoperator and svc-vdm_stage0 are in the mts 
# group (GID 201), so after chmod g+w the directory is writable by our container user.
#
# Usage:
#   export RESULTSDIR=/cpbu/logs/user-logs/...
#   wait-for-artifact-dir
#

TIMEOUT_SECONDS=600
POLL_INTERVAL=5

# Source callback.sh so a failure here can report itself before this step's
# non-zero exit stops the "&&"-chained entrypoint (wait-for-artifact-dir.sh
# && setup-testbed-env.sh && run-e2e.sh) short of run-e2e.sh, which is
# otherwise the only script that ever POSTs to TEST_CALLBACK_URL.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/callback.sh" ]]; then
    # shellcheck source=./callback.sh
    source "${SCRIPT_DIR}/callback.sh"
fi

if [[ -z "$RESULTSDIR" ]]; then
    echo "[wait-for-artifact-dir] ERROR: RESULTSDIR environment variable is not set"
    declare -F report_callback >/dev/null 2>&1 && report_callback "Failed" "RESULTSDIR environment variable is not set"
    exit 1
fi

echo "[wait-for-artifact-dir] Waiting for directory to exist: $RESULTSDIR"
echo "[wait-for-artifact-dir] Timeout: ${TIMEOUT_SECONDS}s ($(( TIMEOUT_SECONDS / 60 )) minutes)"

elapsed=0
while [[ ! -d "$RESULTSDIR" ]]; do
    if [[ $elapsed -ge $TIMEOUT_SECONDS ]]; then
        echo "[wait-for-artifact-dir] ERROR: Timed out after ${TIMEOUT_SECONDS}s waiting for $RESULTSDIR"
        declare -F report_callback >/dev/null 2>&1 && report_callback "Failed" "Timed out after ${TIMEOUT_SECONDS}s waiting for artifact directory ${RESULTSDIR}"
        exit 1
    fi
    sleep $POLL_INTERVAL
    elapsed=$(( elapsed + POLL_INTERVAL ))
    if (( elapsed % 60 == 0 )); then
        echo "[wait-for-artifact-dir] Still waiting... (${elapsed}s / ${TIMEOUT_SECONDS}s)"
    fi
done

echo "[wait-for-artifact-dir] Directory found: $RESULTSDIR (after ${elapsed}s)"
sudo -u svc-vdm_stage0 chmod g+w "$RESULTSDIR"
sudo -u svc-vdm_stage0 chmod -R g+w "$RESULTSDIR" || true
echo "[wait-for-artifact-dir] Made directory group-writable: $RESULTSDIR"
ls -la "$RESULTSDIR"
touch "$RESULTSDIR/.marker.wait-for-artifact-dir.done"
echo "[wait-for-artifact-dir] Marker created: $RESULTSDIR/.marker.wait-for-artifact-dir.done"
exit 0
