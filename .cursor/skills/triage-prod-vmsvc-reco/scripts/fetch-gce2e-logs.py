#!/usr/bin/env python3
"""
fetch-gce2e-logs.py — Collect gce2e failure data for a pipeline build.

Given a prod-vmsvc-reco-* Jenkins build URL, this script:
  1. Reads the pipeline console to find the downstream gce2e build number.
  2. Downloads the JUnit XML and parses every failing test with its actual time.
  3. Looks up the configured timeout for each failure from wcp.yaml.
  4. Downloads the targeted vm-operator log for each failing test suite.
  5. Prints a structured triage summary including:
     - Failure category (EXACT_TIMEOUT / ASSERTION_FAIL / INFRA_ERROR)
     - The relevant wcp.yaml interval key and configured timeout
     - Recommendation: operator-log-only vs full-support-bundle vs escalate

Usage
-----
  python3 fetch-gce2e-logs.py <pipeline-build-url> [--out DIR] [--wcp-yaml PATH]

Examples
--------
  python3 fetch-gce2e-logs.py \
      https://jenkins-vcfwcp.devops.broadcom.net/job/prod-vmsvc-reco-vds/1917

  python3 fetch-gce2e-logs.py \
      https://jenkins-vcfwcp.devops.broadcom.net/job/prod-vmsvc-reco-nsx-vpc/1317 \
      --out /tmp/gce2e-debug --wcp-yaml ~/Documents/personal/vm-operator/test/e2e/vmservice/config/wcp.yaml
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


# ─── WCP.YAML built-in defaults (from test/e2e/vmservice/config/wcp.yaml) ────

WCP_INTERVALS: dict[str, tuple[str, str]] = {
    "wait-virtual-machine-creation":                 ("5m",  "10s"),
    "wait-virtual-machine-deletion":                 ("5m",  "10s"),
    "wait-virtual-machine-condition-update":         ("5m",  "10s"),
    "wait-virtual-machine-powerstate":               ("5m",  "10s"),
    "wait-virtual-machine-resize":                   ("5m",  "10s"),
    "wait-virtual-machine-moid":                     ("5m",  "10s"),
    "wait-virtual-machine-restart-mode-update":      ("2m",  "10s"),
    "wait-virtual-machine-vmip":                     ("5m",  "10s"),
    "wait-virtual-machine-image-creation":           ("10m", "5s"),
    "consistent-virtual-machine-condition":          ("30s", "5s"),
    "wait-virtual-machine-annotation-update":        ("30s", "5s"),
    "wait-virtual-machine-group-deletion":           ("60s", "5s"),
    "wait-virtual-machine-group-condition-update":   ("6m",  "5s"),
    "wait-virtual-machine-snapshot-condition":       ("10m", "5s"),
    "wait-virtual-machine-snapshot-deletion":        ("60s", "2s"),
    "wait-virtual-machine-snapshot-quota-usage":     ("60s", "5s"),
    "wait-virtual-machine-snapshot-related-resource":("60s", "5s"),
    "wait-virtual-machine-snapshot-revert":          ("5m",  "5s"),
    "wait-virtual-machine-snapshot-update":          ("5m",  "5s"),
    "wait-virtual-machine-web-console-request-creation": ("30s", "5s"),
    "wait-virtual-machine-cns-node-batch-attachment-creation": ("10m", "5s"),
    "wait-virtual-machine-cns-node-batch-attachment-deletion": ("5m", "5s"),
    "wait-content-library-name":                     ("2m",  "5s"),
    "wait-storage-class-ready":                      ("5m",  "15s"),
    "wait-namespace-ready":                          ("5m",  "10s"),
    "wait-kms-cluster-ready":                        ("2m",  "5s"),
    "wait-backup-to-complete":                       ("10m", "5s"),
    "wait-virtual-machine-compute-policy-status-update": ("5m", "10s"),
    "wait-policy-evaluation-creation":               ("3m",  "10s"),
    "wait-policy-evaluation-compliant":              ("2m",  "10s"),
    "wait-policy-evaluation-update":                 ("3m",  "10s"),
    "wait-config-map-creation":                      ("3m",  "10s"),
    "wait-secret-creation":                          ("3m",  "10s"),
    "wait-pod-ready":                                ("2m",  "10s"),
    "login-retry-timeout":                           ("5m",  "10s"),
    "wait-subnet-creation":                          ("2m",  "10s"),
    "wait-subnet-deletion":                          ("6m",  "10s"),
    "wait-security-policy-creation":                 ("2m",  "10s"),
    "wait-pvc-attachment":                           ("5m",  "10s"),
    "wait-pvc-deletion":                             ("2m",  "5s"),
    "wait-jumpbox-sshpass-ready":                    ("30m", "10s"),
}


def _parse_duration(s: str) -> float:
    """Convert wcp.yaml duration string like '10m' or '30s' to seconds."""
    s = s.strip()
    if s.endswith("m"):
        return float(s[:-1]) * 60
    if s.endswith("s"):
        return float(s[:-1])
    return float(s)


def _load_wcp_yaml(path: Path) -> dict[str, tuple[str, str]]:
    """Load wcp.yaml intervals section; fall back to built-in defaults."""
    if not path.exists():
        return WCP_INTERVALS

    result = dict(WCP_INTERVALS)
    try:
        import yaml  # type: ignore
        with open(path) as f:
            data = yaml.safe_load(f)
        for key, val in (data or {}).get("intervals", {}).items():
            short_key = key.split("/", 1)[-1]
            if isinstance(val, list) and len(val) == 2:
                result[short_key] = (str(val[0]), str(val[1]))
    except Exception:
        pass
    return result


# ─── Timeout inference: test name → interval key ─────────────────────────────

_TIMEOUT_HINTS: list[tuple[str, str]] = [
    # pattern in test name (lowercase)        →  interval key
    ("snapshot",                              "wait-virtual-machine-snapshot-condition"),
    ("powerstate",                            "wait-virtual-machine-powerstate"),
    ("powered on",                            "wait-virtual-machine-powerstate"),
    ("power on",                              "wait-virtual-machine-powerstate"),
    ("create.*vm|vm.*create|single virtual",  "wait-virtual-machine-creation"),
    ("vmip|ip address",                       "wait-virtual-machine-vmip"),
    ("hardware version|multiwriter",          "wait-virtual-machine-condition-update"),
    ("backfilled|unmanagedvolume",            "wait-virtual-machine-condition-update"),
    ("customization|ovfenv|linuxprep",        "login-retry-timeout"),
    ("ssh|helloworld",                        "login-retry-timeout"),
    ("resize",                                "wait-virtual-machine-resize"),
    ("moid",                                  "wait-virtual-machine-moid"),
    ("publish",                               "wait-virtual-machine-publish-request-condition"),
    ("backup",                                "wait-backup-to-complete"),
    ("encryption|encrypted|encrypt",         "wait-virtual-machine-powerstate"),
    ("pvc|cns|storage",                       "wait-pvc-attachment"),
    ("image",                                 "wait-virtual-machine-image-creation"),
]


def _infer_interval(test_name: str) -> str | None:
    name_lower = test_name.lower()
    for pattern, key in _TIMEOUT_HINTS:
        if re.search(pattern, name_lower):
            return key
    return None


# ─── Failure classification ────────────────────────────────────────────────────

def _classify_failure(message: str, actual_s: float,
                      interval: tuple[str, str] | None) -> tuple[str, str]:
    """
    Return (category, recommendation) where category is one of:
      EXACT_TIMEOUT      — hit the configured Eventually() timeout
      ASSERTION_FAIL     — returned wrong value (assertion mismatch)
      INFRA_ERROR        — infra-level error (OVA, SSH, MethodFault, etc.)
      HANG               — no configured timeout but very long run
    """
    msg = message.lower() if message else ""

    # Hard infra signals
    if any(x in msg for x in ["methodfault", "ova", "import", "transferring file",
                               "internalservererror", "gitclone", "checkout"]):
        return ("INFRA_ERROR",
                "ESCALATE — Infrastructure failure; requires Nimbus/testbed team intervention. "
                "Check Nimbus log URI. No support bundle needed.")

    if "ssh" in msg or "exit error" in msg or "/helloworld" in msg:
        return ("INFRA_ERROR",
                "ESCALATE — SSH/cloud-init race condition. May be a flake or need test fix (add retry). "
                "No support bundle needed for initial triage.")

    if "timed out" in msg or "timeout" in msg:
        if interval:
            configured_s = _parse_duration(interval[0])
            delta = abs(actual_s - configured_s)
            if delta <= 15:
                return ("EXACT_TIMEOUT",
                        "Deep dive — vm-operator log (already in gce2e artifacts). "
                        "Check what condition is stuck. Support bundle only if operator log is unclear.")
            if actual_s > configured_s * 1.5:
                return ("HANG",
                        "ESCALATE — test ran much longer than configured timeout. "
                        "Possible missing hard timeout guard in BeforeAll. "
                        "Check if pipeline was blocked for 21+ hours.")
        return ("EXACT_TIMEOUT",
                "Deep dive — vm-operator log (already in gce2e artifacts).")

    if any(x in msg for x in ["expected\n", "to equal", "to be true", "to be false",
                               "managedentitystatus", "red", "green"]):
        return ("ASSERTION_FAIL",
                "Check test source + vm-operator log. Usually a product state mismatch. "
                "Support bundle only if operator log shows no reconcile errors.")

    if "nil pointer" in msg or "panic" in msg:
        return ("ASSERTION_FAIL",
                "Nil pointer / panic in test or operator. Check operator log for stack trace. "
                "Support bundle if panic is in operator process.")

    if "409" in msg or "conflict" in msg:
        return ("ASSERTION_FAIL",
                "Resource conflict (409). Test is using Update instead of Patch. "
                "No support bundle needed — fix test code.")

    return ("UNKNOWN", "Inspect operator log. Support bundle if condition stuck.")


# ─── HTTP helper (curl-based for internal TLS) ───────────────────────────────

def _curl(*args: str, output_file: str | None = None) -> str:
    cmd = ["curl", "-sf", "--max-time", "120", "--globoff"]
    user  = os.environ.get("JENKINS_USER", "")
    token = os.environ.get("JENKINS_TOKEN", "")
    if user and token:
        cmd += ["-u", f"{user}:{token}"]
    cmd.extend(args)
    if output_file:
        cmd += ["-o", output_file]
        r = subprocess.run(cmd)
        if r.returncode != 0:
            raise RuntimeError(f"curl failed: {' '.join(args)}")
        return ""
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"curl failed (exit {r.returncode}): {' '.join(args)}\n{r.stderr}")
    return r.stdout


def _fetch_json(url: str) -> dict:
    return json.loads(_curl(url))


# ─── Jenkins helpers ──────────────────────────────────────────────────────────

_GCE2E_BUILD_RE = re.compile(
    r"run-guest-clusters-end-to-end-tests\s+#(\d+)"
)
_NIMBUS_LOG_RE = re.compile(r"resultsDir\s+([^\s\"']+)")


def _find_gce2e_build(console: str) -> str | None:
    matches = _GCE2E_BUILD_RE.findall(console)
    return matches[-1] if matches else None


def _find_nimbus_log_uri(console: str) -> str | None:
    """Return the Nimbus log directory path from the pipeline console."""
    m = re.search(r"resultsDir\s+['\"]?(/cpbu/logs/[^\s\"']+)['\"]?", console)
    if m:
        base = m.group(1)
        # Convert to HTTP URL format
        return base.replace(
            "/cpbu/logs/user-logs/wcp-butler",
            "https://uts-logs.lvn.broadcom.net/user-logs/wcp-butler"
        )
    return None


# ─── JUnit parsing ────────────────────────────────────────────────────────────

class TestFailure:
    def __init__(self, classname: str, name: str, time_s: float,
                 message: str, suite_tag: str):
        self.classname  = classname
        self.name       = name
        self.time_s     = time_s
        self.message    = message
        self.suite_tag  = suite_tag


def _parse_junit(xml_text: str) -> list[TestFailure]:
    root = ET.fromstring(xml_text)
    failures: list[TestFailure] = []
    for tc in root.iter("testcase"):
        fail_nodes = list(tc.iter("failure")) + list(tc.iter("error"))
        if not fail_nodes:
            continue
        name      = tc.get("name", "")
        classname = tc.get("classname", "")
        time_s    = float(tc.get("time", "0"))
        message   = (fail_nodes[0].text or "").strip()[:400]
        suite_tag = _test_to_suite_tag(name)
        failures.append(TestFailure(classname, name, time_s, message, suite_tag))
    return failures


_TEST_SUITE_MAP: list[tuple[str, str]] = [
    # pattern in test name (lowercase)   →  gce2e artifact test_logs dir
    ("vm-snapshot|vmsnapshot|snapshot",    "v1alpha5-vmsnapshot"),
    ("vm-encryption|encrypt",             "vm-encryption"),
    ("vm-lcm|vm-lifecycle|lifecycle",     "vm-lcm"),
    ("vm-hardware|multiwriter|hardware",  "vm-hardware"),
    ("vm-guest-customization|ovfenv|linuxprep|customiz", "vm-guest-customization"),
    ("vi-admin|registervm|register",      "vm-networking"),
    ("vm-group",                          "vm-group"),
    ("vm-longevity",                      "vm-longevity"),
    ("vm-networking",                     "vm-networking"),
    ("vm-web-console",                    "vm-web-console"),
    ("vmpub|publish",                     "vmpub"),
]


def _test_to_suite_tag(name: str) -> str:
    lower = name.lower()
    for pattern, tag in _TEST_SUITE_MAP:
        if re.search(pattern, lower):
            return tag
    return "unknown"


# ─── gce2e artifact downloader ───────────────────────────────────────────────

_GCE2E_BASE = "https://jenkins-vcfwcp.devops.broadcom.net/job/run-guest-clusters-end-to-end-tests"
_JUNIT_REL  = "test_logs/junit_vmservice.xml"
_VMOP_LOG_PATTERN = re.compile(r"logs-vmware-system-vmop-controller-manager.*manager\.log")


def _download_gce2e_artifacts(gce2e_build: str, out_dir: Path) -> dict[str, Path]:
    """
    Download JUnit XML and per-suite vm-operator logs from the gce2e build.
    Returns {artifact_relative_path: local_path}.
    """
    base = f"{_GCE2E_BASE}/{gce2e_build}"
    out_dir.mkdir(parents=True, exist_ok=True)

    # 1. artifact list
    artifacts_data = _fetch_json(f"{base}/api/json?tree=artifacts[fileName,relativePath]")
    artifacts = artifacts_data.get("artifacts", [])

    downloaded: dict[str, Path] = {}

    # 2. JUnit XML
    junit_path = out_dir / "junit_vmservice.xml"
    print(f"  Downloading junit_vmservice.xml ...", flush=True)
    _curl(f"{base}/artifact/{_JUNIT_REL}", output_file=str(junit_path))
    downloaded[_JUNIT_REL] = junit_path

    # 3. Per-suite vm-operator logs
    for art in artifacts:
        rel = art["relativePath"]
        if "test_logs" in rel and _VMOP_LOG_PATTERN.search(art["fileName"]):
            # e.g. test/vmservicee2e/test_logs/v1alpha5-vmsnapshot/logs-vmware-system-vmop-*.log
            parts = rel.split("/")
            suite = parts[3] if len(parts) >= 4 else "unknown"
            dest  = out_dir / suite / art["fileName"]
            dest.parent.mkdir(parents=True, exist_ok=True)
            print(f"  Downloading {suite}/{art['fileName']} ...", flush=True)
            _curl(f"{base}/artifact/{rel}", output_file=str(dest))
            downloaded[rel] = dest

    return downloaded


# ─── Main output ──────────────────────────────────────────────────────────────

def _format_duration(s: float) -> str:
    if s >= 60:
        return f"{s / 60:.1f}m"
    return f"{s:.0f}s"


def _print_triage(failures: list[TestFailure],
                  intervals: dict[str, tuple[str, str]],
                  downloaded: dict[str, Path],
                  gce2e_url: str,
                  nimbus_log_uri: str | None) -> None:
    sep = "─" * 70

    print(f"\n{sep}")
    print(f"gce2e build: {gce2e_url}")
    if nimbus_log_uri:
        print(f"Nimbus logs: {nimbus_log_uri}")
    print(f"\n{len(failures)} failing test(s)\n")

    for i, f in enumerate(failures, 1):
        interval_key = _infer_interval(f.name)
        interval     = intervals.get(interval_key, None) if interval_key else None
        cat, rec     = _classify_failure(f.message, f.time_s, interval)

        # vm-operator log for this suite
        vmop_log: Path | None = None
        for rel, path in downloaded.items():
            if f.suite_tag in rel and "manager.log" in rel:
                vmop_log = path
                break

        print(f"[{i}] {f.name[:80]}")
        print(f"     Suite tag   : {f.suite_tag}")
        print(f"     Actual time : {_format_duration(f.time_s)}")

        if interval_key and interval:
            configured_s = _parse_duration(interval[0])
            print(f"     wcp.yaml    : {interval_key}: {interval[0]} poll={interval[1]}"
                  f"  (={configured_s:.0f}s)")
        elif interval_key:
            print(f"     wcp.yaml    : {interval_key} (not found in config)")

        cat_icon = {"EXACT_TIMEOUT": "⏱", "ASSERTION_FAIL": "❌",
                    "INFRA_ERROR": "🔧", "HANG": "🔴", "UNKNOWN": "❓"}.get(cat, "?")
        print(f"     Category    : {cat_icon} {cat}")
        print(f"     Recommend   : {rec}")

        if vmop_log:
            print(f"     vm-op log   : {vmop_log}")

        # Error snippet
        if f.message:
            snippet = f.message[:200].replace("\n", " ↵ ")
            print(f"     Error msg   : {snippet}")
        print()

    print(sep)

    # Quick search hints
    print("\n# Next steps — copy-paste into your shell:\n")
    for i, f in enumerate(failures, 1):
        vmop_log = next(
            (p for rel, p in downloaded.items()
             if f.suite_tag in rel and "manager.log" in rel),
            None
        )
        if vmop_log:
            # Extract VM name hint from test name (short random suffix)
            cat, _ = _classify_failure(f.message, f.time_s, None)
            if cat == "EXACT_TIMEOUT":
                print(f"# [{i}] {f.suite_tag} — scan operator log for stuck condition:")
                print(f'rg "terminal error|Reconciler error|conditions: \\[\\]" "{vmop_log}" | tail -30')
                print()
            elif cat == "ASSERTION_FAIL":
                print(f"# [{i}] {f.suite_tag} — check operator log for mismatch signal:")
                print(f'rg "Reconciler error|terminal error" "{vmop_log}" | tail -20')
                print()

    print(sep)
    print("\n# Support bundle decision guide:")
    print("  EXACT_TIMEOUT → download support bundle (fetch-bundle.py) only if operator log is unclear")
    print("  ASSERTION_FAIL → check test source code first; support bundle rarely needed")
    print("  INFRA_ERROR    → ESCALATE; do NOT dig into support bundle")
    print("  HANG           → ESCALATE; confirm with pipeline team about missing BeforeAll guard")


# ─── Main ──────────────────────────────────────────────────────────────────────

def main() -> None:
    ap = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument(
        "build_url",
        help="Pipeline Jenkins build URL, e.g. https://.../job/prod-vmsvc-reco-vds/1917",
    )
    ap.add_argument(
        "--out",
        default="/tmp/gce2e-debug",
        metavar="DIR",
        help="Directory to store downloaded artifacts (default: /tmp/gce2e-debug)",
    )
    ap.add_argument(
        "--wcp-yaml",
        default=str(Path.home() / "Documents/personal/vm-operator/test/e2e/vmservice/config/wcp.yaml"),
        metavar="PATH",
        help="Path to wcp.yaml for timeout lookup",
    )
    args = ap.parse_args()

    build_url = args.build_url.rstrip("/")
    out_dir   = Path(args.out)
    wcp_path  = Path(args.wcp_yaml).expanduser()

    # 1. Load timeout config
    intervals = _load_wcp_yaml(wcp_path)
    print(f"Loaded {len(intervals)} intervals from wcp.yaml", flush=True)

    # 2. Fetch pipeline console to find gce2e build number
    print(f"Fetching pipeline console for {build_url} ...", flush=True)
    console = _curl(f"{build_url}/consoleText")
    nimbus_uri = _find_nimbus_log_uri(console)

    gce2e_build = _find_gce2e_build(console)
    if not gce2e_build:
        print("ERROR: No run-guest-clusters-end-to-end-tests build found in console.", file=sys.stderr)
        print("  This build may be INFRA-only (testbed creation / Git checkout failure).", file=sys.stderr)
        if nimbus_uri:
            print(f"  Nimbus log: {nimbus_uri}", file=sys.stderr)
        raise SystemExit(1)

    gce2e_url = f"{_GCE2E_BASE}/{gce2e_build}"
    print(f"Found gce2e build: {gce2e_url}", flush=True)

    # 3. Download JUnit + per-suite operator logs
    print(f"Downloading gce2e artifacts → {out_dir}", flush=True)
    downloaded = _download_gce2e_artifacts(gce2e_build, out_dir)

    # 4. Parse JUnit
    junit_path = out_dir / "junit_vmservice.xml"
    if not junit_path.exists():
        print("ERROR: junit_vmservice.xml not found.", file=sys.stderr)
        raise SystemExit(1)

    failures = _parse_junit(junit_path.read_text())
    if not failures:
        print("No test failures found in JUnit XML — all tests passed.")
        raise SystemExit(0)

    # 5. Print triage summary
    _print_triage(failures, intervals, downloaded, gce2e_url, nimbus_uri)


if __name__ == "__main__":
    main()

