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

# Source callback.sh for report_callback. Guarded (rather than an
# unconditional source) so a missing sibling file can't take down the whole
# script before any test output is produced; report_result below already
# no-ops when report_callback isn't defined.
_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "${_SCRIPT_DIR}/callback.sh" ]; then
    # shellcheck source=./callback.sh
    source "${_SCRIPT_DIR}/callback.sh"
fi

# parse_json_report reads a Ginkgo JSON report (written by --json-report) and
# emits a JSON array of per-test results suitable for the subtest_details field
# of a test callback payload. Each element has "testcasename" and "result".
# Ginkgo's report distinguishes a spec that was excluded by the label/focus
# filter (State "skipped" with no Failure) from one that called Skip() with a
# reason at runtime (State "skipped" with a non-empty Failure.Message) or was
# declared PIt/PDescribe (State "pending"); this maps the former to "Not_Run"
# and the latter two to "SKIPPED" so the callback payload distinguishes
# "never attempted" from "test logic decided to skip". Failed specs also get
# a "syndrome" carrying Failure.Message, the callback API's field for a short
# failure synopsis. Failure.Message is Gomega's full failure output (often an
# object diff or YAML dump running well past 1024 characters), so it is
# truncated to the same 1024-character cap the callback API enforces on
# "syndrome" fields — otherwise the callback POST is rejected outright.
parse_json_report() {
    local json_file="${1}"
    jq '[.[0].SpecReports[] |
        ((.ContainerHierarchyTexts + [.LeafNodeText]) | map(select(. != "")) | join(" ")) as $fulltext |
        ("[" + .LeafNodeType + "]" + (if $fulltext != "" then " " + $fulltext else "" end)
            + (if (.LeafNodeLabels | length) > 0 then " [" + (.LeafNodeLabels | join(", ")) + "]" else "" end)
        ) as $name |
        {
            testcasename: $name,
            result: (
                if .State == "failed" then "FAIL"
                elif .State == "passed" then "PASS"
                elif .State == "pending" then "SKIPPED"
                elif .State == "skipped" then
                    (if (.Failure != null and .Failure.Message != "") then "SKIPPED" else "Not_Run" end)
                else "INVALID"
                end
            )
        } + (if .State == "failed" and (.Failure.Message // "") != ""
             then {syndrome: (.Failure.Message[0:1024])}
             else {} end)
    ]' "${json_file}"
}

# report_result reports the overall pass/fail and per-test breakdown via
# report_callback (see callback.sh). Set TEST_CALLBACK_URL in the environment
# to enable reporting; when unset the callback is a no-op (safe for local
# runs).
report_result() {
    local exit_code="${1}"
    local json_report="${2:-}"

    if [ -z "${TEST_CALLBACK_URL:-}" ]; then
        return 0
    fi

    local result
    if [ "${exit_code}" -eq 0 ]; then
        result="Passed"
    else
        result="Failed"
    fi

    # Build per-test breakdown from the JSON report when available
    local subtest_details="[]"
    if [ -f "${json_report}" ]; then
        subtest_details=$(parse_json_report "${json_report}") || subtest_details="[]"
    fi

    # Build a top-level syndrome from the failed subtests' own syndromes, so
    # the overall result carries a short synopsis instead of a bare Failed.
    local syndrome=""
    if [ "${result}" = "Failed" ]; then
        syndrome=$(printf '%s' "${subtest_details}" | jq -r '
            [.[] | select(.result == "FAIL") | (.testcasename + ": " + (.syndrome // "no failure message"))] | join("; ")
        ')
    fi

    if declare -F report_callback >/dev/null 2>&1; then
        report_callback "${result}" "${syndrome}" "${subtest_details}"
    else
        echo "[run-e2e] WARNING: report_callback unavailable (callback.sh not found); skipping callback" >&2
    fi
}

# Inputs from Environment
MODE="${1:-ginkgo}" 
GINKGO_BIN="${GINKGO_BIN:-ginkgo}"
PREBUILT_BIN="${E2E_PREBUILT_BINARY:-}"
ROOT_DIR="${ROOT_DIR:-./}"
ARTIFACT_FOLDER="${E2E_ARTIFACT_FOLDER:-test_logs}"
REPORT_DIR="${ARTIFACT_FOLDER}"
# Ginkgo suite-level timeout. Ginkgo shuts down gracefully when this elapses,
# writing the JSON report and running AfterSuite cleanup before exiting.
# Keep this below the CI container wall-clock limit so the report is written
# before the container scheduler force-kills the process.
GINKGO_TIMEOUT="${GINKGO_TIMEOUT:-2h}"

# Disable color output in CI env.
export GINKGO_NO_COLOR=${TEST_CALLBACK_URL:+TRUE}

# Define the flag prefix based on mode
# Prebuilt binaries require the "--ginkgo." prefix for ginkgo-specific flags
PREFIX=""
[ "$MODE" = "prebuilt" ] && PREFIX="ginkgo."

# 1. Initialize Ginkgo Args with verbosity, timeout, and json-report
# Logic: --[ginkgo.]v --[ginkgo.]timeout=... --[ginkgo.]json-report=...
GINKGO_ARGS=("--${PREFIX}v" "--${PREFIX}timeout=${GINKGO_TIMEOUT}" "--${PREFIX}json-report=${REPORT_DIR}/report.json")

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
report_result "${E2E_EXIT}" "${REPORT_DIR}/report.json"
exit "${E2E_EXIT}"
