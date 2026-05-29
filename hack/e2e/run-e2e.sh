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
################################################################################

set -o errexit
set -o nounset
set -o pipefail

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

# 5. Execute
if [ "$MODE" = "prebuilt" ]; then
    echo "Running E2E tests (prebuilt: $PREBUILT_BIN)..."
    echo "$PREBUILT_BIN $E2E_ARGS ${GINKGO_ARGS[*]}"
    # shellcheck disable=SC2086
    exec "$PREBUILT_BIN" $E2E_ARGS "${GINKGO_ARGS[@]}"
else
    echo "Running E2E tests (ginkgo CLI)..."
    echo "$GINKGO_BIN ${GINKGO_ARGS[*]} ./test/e2e/vmservice/... -- $E2E_ARGS"
    # shellcheck disable=SC2086
    exec "$GINKGO_BIN" "${GINKGO_ARGS[@]}" ./test/e2e/vmservice/... -- $E2E_ARGS
fi
