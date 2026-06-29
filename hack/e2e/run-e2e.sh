#!/usr/bin/env bash

################################################################################
# Script: run-e2e.sh
# Description: Orchestrates E2E testing for vmservice using either the Ginkgo 
#              CLI or a precompiled test binary.
#
# Calling Context:
#   This script is designed to be called from the Makefile via the targets:
#     - 'make test-e2e-ginkgo' (uses ginkgo CLI)
#     - 'make test-e2e-prebuilt' (uses pre-compiled binary)
#
# Usage: ./run-e2e.sh [mode]
#        mode: "ginkgo" (default) - uses ginkgo CLI to compile and run.
#              "prebuilt" - executes an existing compiled test binary.
#
# Environment Variables (usually passed by Makefile):
#   TEST_FOCUS          - Regex to filter specific tests to run.
#   TEST_SKIP           - Regex to skip specific tests.
#   LABEL_FILTER        - Ginkgo label filter expression.
#   FLAKE_ATTEMPTS      - Number of times to retry flaky tests.
#   E2E_NAMESPACE       - The K8s namespace to target for testing.
#   GINKGO_TIMEOUT      - Ginkgo suite timeout (default: 2h). Ginkgo uses this to
#                         shut down gracefully and write the JUnit report. Set this
#                         below the CI container wall-clock limit so Ginkgo exits
#                         cleanly before the container scheduler kills the process.
#   ROOT_DIR            - Project root directory (defaults to ./).
#   GINKGO_BIN          - Path to the ginkgo executable (for ginkgo mode).
#   E2E_PREBUILT_BINARY - Path to the compiled test binary (for prebuilt mode).
#   E2E_ARTIFACT_FOLDER - Directory to store test results/logs.
#   TEST_CALLBACK_URL   - Optional HTTP endpoint to POST the overall result to
#                         once the run completes. No-op when unset.
################################################################################

set -o errexit
set -o nounset
set -o pipefail

# parse_junit_xml reads a JUnit XML file (written by ginkgo --junit-report) and
# emits a JSON array of per-test results suitable for the subtest_details field
# of a test callback payload. Each element has "testcasename" and "result".
# Ginkgo v2 sets a status="passed|failed|skipped" attribute on every <testcase>
# element; this function maps those to the PASS/FAIL/SKIPPED enum values the
# callback API expects. The function uses only jq and sed so that it works in
# the minimal container environment.
parse_junit_xml() {
    local xml_file="${1}"
    grep '<testcase ' "${xml_file}" | \
    while IFS= read -r line; do
        local name status result
        # The leading space before "name=" is required: greedy matching on a
        # bare "name=" would otherwise capture "classname=" instead, since
        # every <testcase> element's classname attribute follows its name
        # attribute and "classname" contains "name" as a substring.
        name=$(printf '%s' "${line}" | sed 's/.* name="\([^"]*\)".*/\1/')
        status=$(printf '%s' "${line}" | sed -n 's/.*status="\([^"]*\)".*/\1/p')
        case "${status}" in
            failed)  result="FAIL"    ;;
            skipped) result="SKIPPED" ;;
            *)       result="PASS"    ;;
        esac
        jq -n --arg n "${name}" --arg r "${result}" \
            '{testcasename: $n, result: $r}'
    done | jq -s '.'
}

# report_result POSTs the overall pass/fail and per-test breakdown to a generic
# callback URL when one is configured in the environment. The variable name is
# intentionally generic so that this public script does not reference any
# specific CI system. Set TEST_CALLBACK_URL in the environment to enable
# reporting; when unset the function is a no-op (safe for local runs).
report_result() {
    local exit_code="${1}"
    local xml_report="${2:-}"

    if [ -z "${TEST_CALLBACK_URL:-}" ]; then
        return 0
    fi

    local result
    if [ "${exit_code}" -eq 0 ]; then
        result="Passed"
    else
        result="Failed"
    fi

    # Build per-test breakdown from the JUnit report when available
    local subtest_details="[]"
    if [ -f "${xml_report}" ]; then
        subtest_details=$(parse_junit_xml "${xml_report}") || subtest_details="[]"
    fi

    local count
    count=$(printf '%s' "${subtest_details}" | jq 'length')
    echo "[run-e2e] Reporting result '${result}' with ${count} subtests to callback URL..."

    local payload
    payload=$(jq -n \
        --arg result "${result}" \
        --argjson subtests "${subtest_details}" \
        '{result: $result, subtest_details: $subtests}')

    curl --silent --show-error --max-time 30 \
        -X POST "${TEST_CALLBACK_URL}" \
        -H "Content-Type: application/json" \
        -d "${payload}" || echo "[run-e2e] WARNING: callback POST failed (ignored)"
}

# Inputs from Environment
MODE="${1:-ginkgo}" 
GINKGO_BIN="${GINKGO_BIN:-ginkgo}"
PREBUILT_BIN="${E2E_PREBUILT_BINARY:-}"
ROOT_DIR="${ROOT_DIR:-./}"
ARTIFACT_FOLDER="${E2E_ARTIFACT_FOLDER:-test_logs}"
REPORT_DIR="${ARTIFACT_FOLDER}"
# Ginkgo suite-level timeout. Ginkgo shuts down gracefully when this elapses,
# writing the JUnit report and running AfterSuite cleanup before exiting.
# Keep this below the CI container wall-clock limit so the report is written
# before the container scheduler force-kills the process.
GINKGO_TIMEOUT="${GINKGO_TIMEOUT:-2h}"

# Define the flag prefix based on mode
# Prebuilt binaries require the "--ginkgo." prefix for ginkgo-specific flags
PREFIX=""
[ "$MODE" = "prebuilt" ] && PREFIX="ginkgo."

# 1. Initialize Ginkgo Args with verbosity, timeout, and junit-report
# Logic: --[ginkgo.]v --[ginkgo.]timeout=... --[ginkgo.]junit-report=...
GINKGO_ARGS=("--${PREFIX}v" "--${PREFIX}timeout=${GINKGO_TIMEOUT}" "--${PREFIX}junit-report=${REPORT_DIR}/test-results.xml")

# 2. Map Environment Variables to Ginkgo Flags
# Syntax: "ENV_VAR_NAME:flag-name"
FLAG_MAP=(
    "TEST_FOCUS:focus"
    "TEST_SKIP:skip"
    "LABEL_FILTER:label-filter"
    "FLAKE_ATTEMPTS:flake-attempts"
)

for pair in "${FLAG_MAP[@]}"; do
    ENV_VAR="${pair%%:*}"
    FLAG_NAME="${pair##*:}"
    
    # Check if the environment variable is set and not empty
    if [ -n "${!ENV_VAR:-}" ]; then
        GINKGO_ARGS+=("--${PREFIX}${FLAG_NAME}=${!ENV_VAR}")
    fi
done

# 3. Handle E2E Namespace
if [ -n "${E2E_NAMESPACE:-}" ]; then
    export E2E_NAMESPACE
fi

# 4. Define E2E specific arguments
E2E_ARGS="-e2e.e2e-config=${ROOT_DIR}test/e2e/vmservice/config/wcp.yaml -e2e.artifactFolder=${ARTIFACT_FOLDER}"

# 5. Execute (capture exit code so we can report it before exiting)
E2E_EXIT=0
if [ "$MODE" = "prebuilt" ]; then
    echo "Running E2E tests (prebuilt: $PREBUILT_BIN)..."
    echo "$PREBUILT_BIN $E2E_ARGS ${GINKGO_ARGS[*]}"
    # shellcheck disable=SC2086
    "$PREBUILT_BIN" $E2E_ARGS "${GINKGO_ARGS[@]}" || E2E_EXIT=$?
else
    echo "Running E2E tests (ginkgo CLI)..."
    echo "$GINKGO_BIN ${GINKGO_ARGS[*]} ./test/e2e/vmservice/... -- $E2E_ARGS"
    # shellcheck disable=SC2086
    "$GINKGO_BIN" "${GINKGO_ARGS[@]}" ./test/e2e/vmservice/... -- $E2E_ARGS || E2E_EXIT=$?
fi

# 6. Report result, then propagate the original exit code
report_result "${E2E_EXIT}" "${REPORT_DIR}/test-results.xml"
exit "${E2E_EXIT}"
