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
        --testbed-info-json) TESTBED_INFO_JSON="$2"; shift 2 ;;
        --testbed-blob-url)  TESTBED_BLOB_URL="$2";  shift 2 ;;
        --output-dir)        OUTPUT_DIR="$2";         shift 2 ;;
        -h|--help)           usage; exit 0 ;;
        *) _err "Unknown argument: $1"; usage; exit 1 ;;
    esac
done

# ---------------------------------------------------------------------------
# Load testbedInfo.json — from a blob URL or a local file
# ---------------------------------------------------------------------------
TESTBED_TMP=""

if [[ -n "${TESTBED_BLOB_URL}" ]]; then
    _log "Fetching testbedInfo from: ${TESTBED_BLOB_URL}"
    TESTBED_TMP="$(mktemp /tmp/testbedInfo.XXXXXX.json)"
    _raw_tmp="$(mktemp /tmp/testbedInfo-raw.XXXXXX.json)"
    if ! curl -sf "${TESTBED_BLOB_URL}" -o "${_raw_tmp}"; then
        _err "Failed to fetch testbedInfo from ${TESTBED_BLOB_URL}"
        rm -f "${_raw_tmp}"
        exit 1
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
    [[ -f "${TESTBED_INFO_JSON}" ]] || { _err "File not found: ${TESTBED_INFO_JSON}"; exit 1; }
else
    _err "Either --testbed-info-json or --testbed-blob-url is required"
    usage; exit 1
fi

cleanup() { [[ -n "${TESTBED_TMP}" && -f "${TESTBED_TMP}" ]] && rm -f "${TESTBED_TMP}"; }
trap cleanup EXIT

_jq() { jq -r "$@" "${TESTBED_INFO_JSON}"; }

# ---------------------------------------------------------------------------
# Parse vCenter credentials
# Supports both VDS (.vc[] array) and VPC (.vc{} object, keyed by runid)
# testbed shapes; .vc[0] only works for the array shape, so pick the first
# entry regardless of which shape is present.
# vimUsername/vimPassword are SSO credentials required by the namespace-management
# API; root is a local vCenter account that lacks namespace-management privileges.
# Some testbed shapes (e.g. VPC) label the same SSO account plain
# username/password instead of vimUsername/vimPassword, so fall back to those.
# ---------------------------------------------------------------------------
IFS=$'\t' read -r VC_IP VC_VIM_USER VC_VIM_PWD < <(_jq '
    def firstvc:
      if (.vc | type) == "array" then .vc[0]
      else (.vc | to_entries | sort_by(.key | tonumber) | .[0].value)
      end;
    # username/password fall back to the plain (non-"root") account when
    # vimUsername/vimPassword are absent; never fall back to root itself.
    def ssouser: firstvc.vimUsername // (if firstvc.username == "root" then empty else firstvc.username end);
    [
      firstvc.ip4 // firstvc.ip // "",
      ssouser // "",
      (if firstvc.vimUsername then firstvc.vimPassword else (if firstvc.username == "root" then empty else firstvc.password end) end) // ""
    ] | @tsv
')

if [[ -z "${VC_IP}" || -z "${VC_VIM_USER}" || -z "${VC_VIM_PWD}" ]]; then
    _err "Could not extract VC IP or vim credentials from testbedInfo.json"
    exit 1
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

SUPERVISOR_ID=""
CLUSTER_ID=""

# ---------------------------------------------------------------------------
# v2 API: supervisors endpoint (vSphere 8.0+)
# ---------------------------------------------------------------------------
SUPERVISOR_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/vcenter/namespace-management/supervisors" \
    | jq -r 'if type == "array" then .[0].supervisor // empty else empty end')

if [[ -z "${SUPERVISOR_ID}" ]]; then
    # ---------------------------------------------------------------------------
    # v1 fallback: clusters endpoint (pre-8.0)
    # ---------------------------------------------------------------------------
    CLUSTER_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
        "https://${VC_IP}/api/vcenter/namespace-management/clusters" \
        | jq -r 'if type == "array" then .[0].cluster // empty else empty end')
fi

if [[ -z "${SUPERVISOR_ID}" && -z "${CLUSTER_ID}" ]]; then
    _warn "No supervisor or cluster found on VC ${VC_IP}; skipping WCP bundle"
fi

# ---------------------------------------------------------------------------
# Request and download the bundle, with retries.
#
# The support-bundle token is single-use (VC discards it once a download is
# attempted), so a retry must re-request a fresh bundle/token — it cannot
# just re-download the previous URL. Each attempt is validated for both a
# successful HTTP status and a structurally-intact tar before being accepted;
# a truncated download (e.g. VC's own bundle-generation timing out mid-stream)
# is otherwise silently treated as success because curl only reports failure
# on transport-level errors, not on a short/incomplete 2xx response body.
# ---------------------------------------------------------------------------
BUNDLE_OUT="${OUTPUT_DIR}/wcp-support-bundle.tar"
MAX_ATTEMPTS=3
DOWNLOAD_OK=false

if [[ -n "${SUPERVISOR_ID}" || -n "${CLUSTER_ID}" ]]; then
    for attempt in $(seq 1 "${MAX_ATTEMPTS}"); do
        BUNDLE_URL=""
        BUNDLE_TOKEN=""

        if [[ -n "${SUPERVISOR_ID}" ]]; then
            _log "Getting WCP bundle for supervisor ${SUPERVISOR_ID} (v2 API), attempt ${attempt}/${MAX_ATTEMPTS}..."
            BUNDLE_INFO=$(curl -sk --max-time 30 -X POST \
                -H "vmware-api-session-id: ${VC_SESSION}" \
                "https://${VC_IP}/api/vcenter/namespace-management/supervisors/${SUPERVISOR_ID}/support-bundles")
            BUNDLE_URL=$(echo "${BUNDLE_INFO}" | jq -r '.url // empty')
            BUNDLE_TOKEN=$(echo "${BUNDLE_INFO}" | jq -r '.support_bundle_token.token // empty')
        else
            _log "Getting WCP bundle for cluster ${CLUSTER_ID} (v1 API), attempt ${attempt}/${MAX_ATTEMPTS}..."
            BUNDLE_INFO=$(curl -sk --max-time 30 -X POST \
                -H "vmware-api-session-id: ${VC_SESSION}" \
                "https://${VC_IP}/api/vcenter/namespace-management/clusters/${CLUSTER_ID}/support-bundle" \
                || echo '{}')
            BUNDLE_URL=$(echo "${BUNDLE_INFO}" | jq -r '.url // empty')
            BUNDLE_TOKEN=$(echo "${BUNDLE_INFO}" | jq -r '.wcp_support_bundle_token.token // empty')
        fi

        if [[ -z "${BUNDLE_URL}" || -z "${BUNDLE_TOKEN}" ]]; then
            _warn "Attempt ${attempt}/${MAX_ATTEMPTS}: could not obtain WCP support bundle URL or token"
            sleep 5
            continue
        fi

        _log "Downloading WCP support bundle from ${BUNDLE_URL}..."
        WCP_BUNDLE_PAYLOAD="{\"wcp-support-bundle-token\": \"${BUNDLE_TOKEN}\"}"
        _bundle_http=$(curl -sk --max-time 600 -X POST \
            -H 'Content-Type: application/json' \
            -d "${WCP_BUNDLE_PAYLOAD}" \
            "${BUNDLE_URL}" \
            -o "${BUNDLE_OUT}" \
            -w '%{http_code}')
        _curl_rc=$?

        if [[ ${_curl_rc} -ne 0 ]]; then
            _warn "Attempt ${attempt}/${MAX_ATTEMPTS}: WCP support bundle download failed (curl exit ${_curl_rc})"
        elif [[ "${_bundle_http}" != "20"* ]]; then
            _warn "Attempt ${attempt}/${MAX_ATTEMPTS}: WCP support bundle download failed (HTTP ${_bundle_http})"
        elif ! tar -tf "${BUNDLE_OUT}" >/dev/null 2>&1; then
            _warn "Attempt ${attempt}/${MAX_ATTEMPTS}: downloaded WCP support bundle is truncated or not a valid tar"
        else
            _log "WCP support bundle downloaded and verified on attempt ${attempt}/${MAX_ATTEMPTS}"
            DOWNLOAD_OK=true
            break
        fi

        rm -f "${BUNDLE_OUT}"
        [[ ${attempt} -lt ${MAX_ATTEMPTS} ]] && sleep 15
    done

    if [[ "${DOWNLOAD_OK}" != true ]]; then
        _warn "WCP support bundle download did not succeed after ${MAX_ATTEMPTS} attempts; leaving ${OUTPUT_DIR} without a bundle"
    fi
fi

curl -sk -X DELETE \
    -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/session" > /dev/null 2>&1 || true

_log "Done. Output: ${OUTPUT_DIR}"
