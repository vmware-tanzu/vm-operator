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
#
# This is a best-effort diagnostic step, not a test: any runtime failure to
# obtain or download a bundle exits 0 so it never fails the caller's
# pipeline. Only invocation errors (bad/missing flags) exit non-zero.

set -uo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
_log()  { echo "[${SCRIPT_NAME}] $*"; }
_warn() { echo "[${SCRIPT_NAME}] WARNING: $*" >&2; }
_err()  { echo "[${SCRIPT_NAME}] ERROR: $*" >&2; }

TESTBED_INFO_JSON=""
TESTBED_BLOB_URL=""
OUTPUT_DIR="${RESULTSDIR:-/tmp/support-bundles}"

# WCP allows only one support-bundle generation in flight per supervisor/
# cluster. Multiple pipelines (or multiple test suites within one pipeline,
# or a per-suite inline collection racing the standalone collect-support-
# bundle job) can request a bundle from the same VC around the same time.
# When that happens, a losing request can still get back HTTP 2xx with a
# url/token, but downloading it yields an empty body instead of an outright
# rejection - so retries cover both the request and the download, not just
# the request. A real generation+download has been observed to take ~9
# minutes end to end, so the default budget (attempts * interval, not
# counting per-attempt request/download time) targets ~15 minutes to
# comfortably outlast one competing collection. This must stay under the
# calling UTS job's timeout with headroom for the rest of the job.
SUPPORT_BUNDLE_RETRY_ATTEMPTS="${SUPPORT_BUNDLE_RETRY_ATTEMPTS:-30}"
SUPPORT_BUNDLE_RETRY_INTERVAL_SECONDS="${SUPPORT_BUNDLE_RETRY_INTERVAL_SECONDS:-30}"

# Download failures (timeouts, resets, damaged archives) are budgeted
# separately from "busy" responses. Each download attempt regenerates the whole
# bundle, so this is kept small; the partial copy from the last failure is
# still preserved.
SUPPORT_BUNDLE_DOWNLOAD_ATTEMPTS="${SUPPORT_BUNDLE_DOWNLOAD_ATTEMPTS:-2}"

# The bundle is generated and streamed within the same download request, so
# this cap must cover generation plus transfer. Generation alone takes ~9
# minutes on a small testbed and longer with more hosts; a 600s cap was
# observed to truncate the stream mid-transfer.
SUPPORT_BUNDLE_DOWNLOAD_TIMEOUT_SECONDS="${SUPPORT_BUNDLE_DOWNLOAD_TIMEOUT_SECONDS:-1800}"

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
        _warn "Failed to fetch testbedInfo from ${TESTBED_BLOB_URL}; skipping WCP bundle"
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
    [[ -f "${TESTBED_INFO_JSON}" ]] || { _warn "File not found: ${TESTBED_INFO_JSON}; skipping WCP bundle"; exit 0; }
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
    _warn "Could not extract VC IP or vim credentials from testbedInfo.json; skipping WCP bundle"
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

OUT_FILE="${OUTPUT_DIR}/wcp-support-bundle.tar"
# A failed download may still have streamed a useful prefix of the archive,
# so it is kept here (overwritten by each failed attempt) rather than deleted.
PARTIAL_FILE="${OUTPUT_DIR}/wcp-support-bundle.partial.tar"

# ---------------------------------------------------------------------------
# Request a bundle and download it, in one attempt. Returns:
#   0  bundle downloaded and readable (OUT_FILE written)
#   1  busy: another collection holds the generation slot (trigger rejected,
#      no url/token, or the download came back empty); nothing to keep
#   2  download failed after data or a connection error; a partial copy is
#      kept at PARTIAL_FILE when any bytes were received
# ---------------------------------------------------------------------------
try_collect_bundle() {
    local trigger_url="$1" token_field="$2"
    local response http_code body url token dl_tmp dl_http_code dl_rc

    response=$(curl -sk -w '\n%{http_code}' --max-time 30 -X POST \
        -H "vmware-api-session-id: ${VC_SESSION}" "${trigger_url}")
    http_code=$(printf '%s' "${response}" | tail -1)
    body=$(printf '%s' "${response}" | sed '$d')

    if [[ "${http_code}" != "20"* ]]; then
        _warn "Support bundle request to ${trigger_url} did not return a bundle (HTTP ${http_code}); another collection may be in progress"
        return 1
    fi

    url=$(printf '%s' "${body}" | jq -r '.url // empty')
    token=$(printf '%s' "${body}" | jq -r "${token_field}")
    if [[ -z "${url}" || -z "${token}" ]]; then
        _warn "Support bundle request to ${trigger_url} returned no url/token"
        return 1
    fi

    _log "Downloading WCP support bundle from ${url}..."
    dl_tmp="$(mktemp "${OUTPUT_DIR}/wcp-support-bundle.XXXXXX.tar")"
    dl_http_code=$(curl -sk -w '%{http_code}' \
        --max-time "${SUPPORT_BUNDLE_DOWNLOAD_TIMEOUT_SECONDS}" -X POST \
        -H 'Content-Type: application/json' \
        -d "{\"wcp-support-bundle-token\": \"${token}\"}" \
        "${url}" -o "${dl_tmp}")
    dl_rc=$?

    if [[ ! -s "${dl_tmp}" ]]; then
        rm -f "${dl_tmp}"
        if (( dl_rc != 0 )); then
            _warn "Download of support bundle from ${url} failed (curl exit ${dl_rc}, HTTP ${dl_http_code}) with no data"
            return 2
        fi
        _warn "Downloaded support bundle from ${url} was empty (HTTP ${dl_http_code}); another collection likely won the race"
        return 1
    fi

    # Only a complete transfer that reads back as a tar counts as success.
    if (( dl_rc != 0 )) || ! tar -tf "${dl_tmp}" >/dev/null 2>&1; then
        _warn "Support bundle from ${url} is incomplete (curl exit ${dl_rc}, HTTP ${dl_http_code}); keeping partial copy at ${PARTIAL_FILE}"
        mv -f "${dl_tmp}" "${PARTIAL_FILE}"
        chmod 644 "${PARTIAL_FILE}"
        return 2
    fi

    mv "${dl_tmp}" "${OUT_FILE}"
    chmod 644 "${OUT_FILE}"
    return 0
}

# ---------------------------------------------------------------------------
# v2 API: supervisors endpoint (vSphere 8.0+); v1 fallback: clusters endpoint
# (pre-8.0). The endpoint choice only depends on which resource exists on
# this VC, so it is resolved once, outside the retry loop below.
# ---------------------------------------------------------------------------
SUPERVISOR_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/vcenter/namespace-management/supervisors" \
    | jq -r 'if type == "array" then .[0].supervisor // empty else empty end')

if [[ -n "${SUPERVISOR_ID}" ]]; then
    _log "Getting WCP bundle for supervisor ${SUPERVISOR_ID} (v2 API)..."
    TRIGGER_URL="https://${VC_IP}/api/vcenter/namespace-management/supervisors/${SUPERVISOR_ID}/support-bundles"
    TOKEN_FIELD='.support_bundle_token.token // empty'
else
    CLUSTER_ID=$(curl -sk --max-time 30 -H "vmware-api-session-id: ${VC_SESSION}" \
        "https://${VC_IP}/api/vcenter/namespace-management/clusters" \
        | jq -r 'if type == "array" then .[0].cluster // empty else empty end')
    if [[ -n "${CLUSTER_ID}" ]]; then
        _log "Getting WCP bundle for cluster ${CLUSTER_ID} (v1 API)..."
        TRIGGER_URL="https://${VC_IP}/api/vcenter/namespace-management/clusters/${CLUSTER_ID}/support-bundle"
        TOKEN_FIELD='.wcp_support_bundle_token.token // empty'
    else
        _warn "No supervisor or cluster found on VC ${VC_IP}; skipping WCP bundle"
        TRIGGER_URL=""
        TOKEN_FIELD=""
    fi
fi

if [[ -n "${TRIGGER_URL}" ]]; then
    # Busy waits and download failures are counted separately: waiting on
    # another collection should not consume the download budget, and a slow
    # download should not consume the wait budget.
    busy_count=0
    download_count=0
    while true; do
        rc=0
        try_collect_bundle "${TRIGGER_URL}" "${TOKEN_FIELD}" || rc=$?
        (( rc == 0 )) && break

        if (( rc == 1 )); then
            busy_count=$((busy_count + 1))
            if (( busy_count >= SUPPORT_BUNDLE_RETRY_ATTEMPTS )); then
                _warn "Gave up waiting for a WCP support bundle from ${VC_IP} after ${busy_count} busy attempts"
                break
            fi
            _warn "Retrying support bundle collection in ${SUPPORT_BUNDLE_RETRY_INTERVAL_SECONDS}s (busy attempt ${busy_count}/${SUPPORT_BUNDLE_RETRY_ATTEMPTS})..."
        else
            download_count=$((download_count + 1))
            if (( download_count >= SUPPORT_BUNDLE_DOWNLOAD_ATTEMPTS )); then
                _warn "Gave up downloading a WCP support bundle from ${VC_IP} after ${download_count} failed downloads"
                break
            fi
            _warn "Retrying support bundle download in ${SUPPORT_BUNDLE_RETRY_INTERVAL_SECONDS}s (download attempt ${download_count}/${SUPPORT_BUNDLE_DOWNLOAD_ATTEMPTS})..."
        fi
        sleep "${SUPPORT_BUNDLE_RETRY_INTERVAL_SECONDS}"
    done
fi

curl -sk -X DELETE \
    -H "vmware-api-session-id: ${VC_SESSION}" \
    "https://${VC_IP}/api/session" > /dev/null 2>&1 || true

_log "Done. Output: ${OUTPUT_DIR}"
