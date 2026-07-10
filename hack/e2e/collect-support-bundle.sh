#!/usr/bin/env bash
# Collects the WCP supervisor support bundle from a testbed vCenter.
#
# Uses the vSphere REST API to authenticate, locate the supervisor or cluster,
# request a support bundle, and download it to the output directory.
#
# Usage (local testbedInfo.json):
#   collect-support-bundle.sh --testbed-info-json ./testbedInfo.json --output-dir /logs
#
# Usage (UTS blob URL — unwraps deliverable_blob automatically):
#   collect-support-bundle.sh --testbed-blob-url "$BLOB_URL" --output-dir "$RESULTSDIR"
#
# Environment variable overrides:
#   RESULTSDIR   Default output directory when --output-dir is not passed.

set -uo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
_log()  { echo "[${SCRIPT_NAME}] $*"; }
_warn() { echo "[${SCRIPT_NAME}] WARNING: $*" >&2; }
_err()  { echo "[${SCRIPT_NAME}] ERROR: $*" >&2; }

TESTBED_INFO_JSON=""
TESTBED_BLOB_URL=""
OUTPUT_DIR="${RESULTSDIR:-/tmp/support-bundles}"
_LOCATION_FLAG_GIVEN=false

usage() {
    cat >&2 <<EOF
Usage: ${SCRIPT_NAME} [OPTIONS]

Options:
  --testbed-info-json <file>   Path to a local testbedInfo.json file
  --testbed-blob-url <url>     UTS blob URL; fetches and unwraps deliverable_blob
  --output-dir <dir>           Directory for output files (default: \$RESULTSDIR)
  -h, --help                   Show this help and exit

Examples:
  ${SCRIPT_NAME} --testbed-info-json ./testbedInfo.json --output-dir /logs
  ${SCRIPT_NAME} --testbed-blob-url "\${BLOB_URL}" --output-dir "\${RESULTSDIR}"
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --testbed-info-json) TESTBED_INFO_JSON="$2"; _LOCATION_FLAG_GIVEN=true; shift 2 ;;
        --testbed-blob-url)  TESTBED_BLOB_URL="$2";  _LOCATION_FLAG_GIVEN=true; shift 2 ;;
        --output-dir)        OUTPUT_DIR="$2";         shift 2 ;;
        -h|--help)           usage; exit 0 ;;
        *) _err "Unknown argument: $1"; usage; exit 1 ;;
    esac
done

# Fail open: a testbed that was never provisioned (e.g. the provisioning
# workload itself failed or didn't run) has no support bundle to collect.
# Only treat a missing --testbed-info-json/--testbed-blob-url flag entirely
# (a real invocation mistake) as a usage error; an empty value for a flag
# that WAS passed means "no testbed" and should be a graceful no-op.
if [[ "${_LOCATION_FLAG_GIVEN}" == "true" && -z "${TESTBED_INFO_JSON}" && -z "${TESTBED_BLOB_URL}" ]]; then
    _warn "No testbed location provided (empty --testbed-info-json/--testbed-blob-url); testbed was likely never provisioned. Skipping support bundle collection."
    exit 0
fi

# ---------------------------------------------------------------------------
# Load testbedInfo.json — from a blob URL or a local file
# ---------------------------------------------------------------------------
TESTBED_TMP=""

if [[ -n "${TESTBED_BLOB_URL}" ]]; then
    _log "Fetching testbedInfo from: ${TESTBED_BLOB_URL}"
    TESTBED_TMP="$(mktemp /tmp/testbedInfo.XXXXXX.json)"
    _raw_tmp="$(mktemp /tmp/testbedInfo-raw.XXXXXX.json)"
    if ! curl -sf "${TESTBED_BLOB_URL}" -o "${_raw_tmp}"; then
        _warn "Testbed data at ${TESTBED_BLOB_URL} is not accessible (testbed likely never provisioned); skipping support bundle collection."
        rm -f "${_raw_tmp}"
        exit 0
    fi
    # Unwrap deliverable_blob if present (UTS test_blob API format); otherwise
    # the URL points directly to the raw testbedInfo.json in the logs directory.
    if jq -e '.deliverable_blob' "${_raw_tmp}" >/dev/null 2>&1; then
        jq -r '.deliverable_blob' "${_raw_tmp}" > "${TESTBED_TMP}"
    else
        cp "${_raw_tmp}" "${TESTBED_TMP}"
    fi
    rm -f "${_raw_tmp}"
    TESTBED_INFO_JSON="${TESTBED_TMP}"
elif [[ -n "${TESTBED_INFO_JSON}" ]]; then
    if [[ ! -f "${TESTBED_INFO_JSON}" ]]; then
        _warn "Testbed info file ${TESTBED_INFO_JSON} not found (testbed likely never provisioned); skipping support bundle collection."
        exit 0
    fi
else
    _err "Either --testbed-info-json or --testbed-blob-url is required"
    usage; exit 1
fi

cleanup() { [[ -n "${TESTBED_TMP}" && -f "${TESTBED_TMP}" ]] && rm -f "${TESTBED_TMP}"; }
trap cleanup EXIT

_jq() { jq -r "$@" "${TESTBED_INFO_JSON}"; }

# ---------------------------------------------------------------------------
# Parse vCenter credentials
# Supports both VDS (.vc[] array) and VPC (.vc{} object) testbed shapes.
# vimUsername/vimPassword are SSO credentials required by the namespace-management
# API; root is a local vCenter account that lacks namespace-management privileges.
# ---------------------------------------------------------------------------
VC_IP="$(_jq '.vc[0].ip4 // .vc[0].ip // empty')"
VC_VIM_USER="$(_jq '.vc[0].vimUsername // empty')"
VC_VIM_PWD="$(_jq '.vc[0].vimPassword // empty')"

if [[ -z "${VC_IP}" || -z "${VC_VIM_USER}" || -z "${VC_VIM_PWD}" ]]; then
    _warn "Could not extract VC IP or vim credentials from testbedInfo.json; testbed is not accessible. Skipping support bundle collection."
    exit 0
fi

_log "Collecting WCP support bundle from VC ${VC_IP}..."
mkdir -p "${OUTPUT_DIR}"

# ---------------------------------------------------------------------------
# Authenticate to vCenter REST API
# POST /api/session returns 201 Created on success.
# ---------------------------------------------------------------------------
_vc_session_response=$(curl -sk -w '\n%{http_code}' -X POST \
    -u "${VC_VIM_USER}:${VC_VIM_PWD}" \
    "https://${VC_IP}/api/session")
_vc_session_http=$(printf '%s' "${_vc_session_response}" | tail -1)
VC_SESSION=$(printf '%s' "${_vc_session_response}" | head -1 | tr -d '"')

if [[ "${_vc_session_http}" != "20"* || -z "${VC_SESSION}" || "${VC_SESSION}" == "null" ]]; then
    _warn "Could not authenticate to vCenter REST API on ${VC_IP} (HTTP ${_vc_session_http}); skipping WCP bundle"
    exit 0
fi

BUNDLE_URL=""
BUNDLE_TOKEN=""

# ---------------------------------------------------------------------------
# v2 API: supervisors endpoint (vSphere 8.0+)
# ---------------------------------------------------------------------------
SUPERVISOR_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/vcenter/namespace-management/supervisors" \
    | jq -r 'if type == "array" then .[0].supervisor // empty else empty end')

if [[ -n "${SUPERVISOR_ID}" ]]; then
    _log "Getting WCP bundle for supervisor ${SUPERVISOR_ID} (v2 API)..."
    BUNDLE_INFO=$(curl -sk --max-time 30 -X POST \
        -H "vmware-api-session-id: ${VC_SESSION}" \
        "https://${VC_IP}/api/vcenter/namespace-management/supervisors/${SUPERVISOR_ID}/support-bundles")
    BUNDLE_URL=$(echo "${BUNDLE_INFO}" | jq -r '.url // empty')
    BUNDLE_TOKEN=$(echo "${BUNDLE_INFO}" | jq -r '.support_bundle_token.token // empty')
else
    # ---------------------------------------------------------------------------
    # v1 fallback: clusters endpoint (pre-8.0)
    # ---------------------------------------------------------------------------
    CLUSTER_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
        "https://${VC_IP}/api/vcenter/namespace-management/clusters" \
        | jq -r 'if type == "array" then .[0].cluster // empty else empty end')
    if [[ -n "${CLUSTER_ID}" ]]; then
        _log "Getting WCP bundle for cluster ${CLUSTER_ID} (v1 API)..."
        BUNDLE_INFO=$(curl -sk --max-time 30 -X POST \
            -H "vmware-api-session-id: ${VC_SESSION}" \
            "https://${VC_IP}/api/vcenter/namespace-management/clusters/${CLUSTER_ID}/support-bundle" \
            || echo '{}')
        BUNDLE_URL=$(echo "${BUNDLE_INFO}" | jq -r '.url // empty')
        BUNDLE_TOKEN=$(echo "${BUNDLE_INFO}" | jq -r '.wcp_support_bundle_token.token // empty')
    else
        _warn "No supervisor or cluster found on VC ${VC_IP}; skipping WCP bundle"
    fi
fi

# ---------------------------------------------------------------------------
# Download the bundle
# ---------------------------------------------------------------------------
if [[ -n "${BUNDLE_URL}" && -n "${BUNDLE_TOKEN}" ]]; then
    _log "Downloading WCP support bundle from ${BUNDLE_URL}..."
    WCP_BUNDLE_PAYLOAD="{\"wcp-support-bundle-token\": \"${BUNDLE_TOKEN}\"}"
    curl -sk --max-time 600 -X POST \
        -H 'Content-Type: application/json' \
        -d "${WCP_BUNDLE_PAYLOAD}" \
        "${BUNDLE_URL}" \
        -o "${OUTPUT_DIR}/wcp-support-bundle.tar" \
        || _warn "WCP support bundle download failed"
else
    _warn "Could not obtain WCP support bundle URL or token"
fi

curl -sk -X DELETE \
    -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/session" > /dev/null 2>&1 || true

_log "Done. Output: ${OUTPUT_DIR}"
