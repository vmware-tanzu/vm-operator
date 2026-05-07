#!/bin/bash
set -e

#
# wait-for-artifact-dir.sh - Wait for E2E_ARTIFACT_FOLDER to exist and become writable
#
# Spins until the directory specified by E2E_ARTIFACT_FOLDER exists, then
# makes it group-writable so that tests can write logs and artifacts into it.
# Exits 1 on timeout (10 minutes).
#
# Usage:
#   export E2E_ARTIFACT_FOLDER=/path/to/artifacts
#   wait-for-artifact-dir.sh
#

TIMEOUT_SECONDS=600
POLL_INTERVAL=5

if [[ -z "$E2E_ARTIFACT_FOLDER" ]]; then
    echo "[wait-for-artifact-dir] ERROR: E2E_ARTIFACT_FOLDER is not set"
    exit 1
fi

echo "[wait-for-artifact-dir] Waiting for artifact directory: $E2E_ARTIFACT_FOLDER"
echo "[wait-for-artifact-dir] Timeout: ${TIMEOUT_SECONDS}s ($(( TIMEOUT_SECONDS / 60 )) minutes)"

elapsed=0
while [[ ! -d "$E2E_ARTIFACT_FOLDER" ]]; do
    if [[ $elapsed -ge $TIMEOUT_SECONDS ]]; then
        echo "[wait-for-artifact-dir] ERROR: Timed out after ${TIMEOUT_SECONDS}s waiting for $E2E_ARTIFACT_FOLDER"
        exit 1
    fi
    sleep $POLL_INTERVAL
    elapsed=$(( elapsed + POLL_INTERVAL ))
    if (( elapsed % 60 == 0 )); then
        echo "[wait-for-artifact-dir] Still waiting... (${elapsed}s / ${TIMEOUT_SECONDS}s)"
    fi
done

echo "[wait-for-artifact-dir] Directory found (after ${elapsed}s)"
ls -la "$E2E_ARTIFACT_FOLDER"

sudo chmod g+w "$E2E_ARTIFACT_FOLDER"
sudo chmod -R g+w "$E2E_ARTIFACT_FOLDER" || true
echo "[wait-for-artifact-dir] Made directory group-writable"
ls -la "$E2E_ARTIFACT_FOLDER"

exit 0
