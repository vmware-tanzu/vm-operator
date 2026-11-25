#!/usr/bin/env bash
#
# callback.sh - Shared helper for reporting E2E pipeline results to a generic
# HTTP callback URL.
#
# wait-for-artifact-dir.sh, setup-testbed-env.sh, and run-e2e.sh are chained
# with "&&" in the E2E container's entrypoint, so a failure in any one of the
# earlier steps stops the chain before run-e2e.sh (previously the only
# script that ever POSTed a result) gets a chance to run. Sourcing this file
# lets every step in the chain report its own "Failed" + syndrome instead of
# leaving the callback endpoint with nothing but the container's bare
# non-zero exit code.
#
# Usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/callback.sh"
#   report_callback "Failed" "SSH to vCenter failed after 6 attempts"
#   report_callback "Passed" "" "${subtest_details_json}"
#
# Set TEST_CALLBACK_URL in the environment to enable reporting; when unset,
# report_callback is a no-op (safe for local runs).

# report_callback posts {result, subtest_details, syndrome} to
# TEST_CALLBACK_URL. "syndrome" and "subtest_details" are optional:
# "syndrome" is omitted from the payload when empty, and "subtest_details"
# defaults to "[]". The callback API caps "syndrome" at 1024 characters.
report_callback() {
    local result="${1}"
    local syndrome="${2:-}"
    local subtest_details="${3:-[]}"

    if [ -z "${TEST_CALLBACK_URL:-}" ]; then
        return 0
    fi

    syndrome=$(printf '%s' "${syndrome}" | cut -c1-1024)

    local count
    count=$(printf '%s' "${subtest_details}" | jq 'length' 2>/dev/null || echo 0)
    echo "[callback] Reporting result '${result}' with ${count} subtests to callback URL..."

    local payload
    payload=$(jq -n \
        --arg result "${result}" \
        --arg syndrome "${syndrome}" \
        --argjson subtests "${subtest_details}" \
        '{result: $result, subtest_details: $subtests}
         + (if $syndrome != "" then {syndrome: $syndrome} else {} end)')

    curl --silent --show-error --max-time 30 \
        -X POST "${TEST_CALLBACK_URL}" \
        -H "Content-Type: application/json" \
        -d "${payload}" || echo "[callback] WARNING: callback POST failed (ignored)"
}
