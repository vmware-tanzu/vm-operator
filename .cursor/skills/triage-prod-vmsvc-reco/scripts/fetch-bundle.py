#!/usr/bin/env python3
"""
fetch-bundle.py — Download and extract a WCP support bundle from Jenkins.

Downloads the wcp-support-*.tar artifact for a given build, extracts only the
master-vm-*.tgz (Supervisor VM logs + pod logs), and prints shell variables
pointing at the vm-operator, NSX-operator, and CSI pod-log directories.

Works without any third-party dependencies (stdlib only).
Compatible with Cursor agent and Claude Code.

Usage
-----
  python3 fetch-bundle.py <jenkins-build-url> [--out DIR]

Examples
--------
  # VDS build
  python3 fetch-bundle.py \
      https://jenkins-vcfwcp.devops.broadcom.net/job/prod-vmsvc-reco-vds/1917

  # NSX-VPC build, custom output dir
  python3 fetch-bundle.py \
      https://jenkins-vcfwcp.devops.broadcom.net/job/prod-vmsvc-reco-nsx-vpc/1317 \
      --out /tmp/wcp-sb-vpc

Authentication
--------------
  No auth needed for this Jenkins instance (public read access).
  If a 401 is returned, set JENKINS_USER and JENKINS_TOKEN in the environment:

      export JENKINS_USER=myuser
      export JENKINS_TOKEN=my-api-token
      python3 fetch-bundle.py ...
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path


# ─── Jenkins URL helpers ───────────────────────────────────────────────────────

_API_SUFFIX  = "/api/json?tree=artifacts[fileName,relativePath]"
_ART_SUFFIX  = "/artifact/{rel}"


def _curl_base() -> list[str]:
    """Build base curl command with optional auth and silent flags."""
    cmd = ["curl", "-sf", "--max-time", "600", "--globoff"]
    user  = os.environ.get("JENKINS_USER", "")
    token = os.environ.get("JENKINS_TOKEN", "")
    if user and token:
        cmd += ["-u", f"{user}:{token}"]
    return cmd


def _fetch_json(url: str) -> dict:
    """Fetch a JSON endpoint via curl (uses system keychain for TLS)."""
    result = subprocess.run([*_curl_base(), url], capture_output=True, text=True)
    if result.returncode != 0:
        print(f"ERROR: curl failed (exit {result.returncode}) for {url}", file=sys.stderr)
        if "401" in result.stderr or "403" in result.stderr:
            print("  Set JENKINS_USER + JENKINS_TOKEN env vars for auth.", file=sys.stderr)
        raise SystemExit(1)
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as e:
        print(f"ERROR: Invalid JSON from {url}: {e}", file=sys.stderr)
        raise SystemExit(1)


def _find_bundle_artifact(build_url: str) -> str:
    """Return the relativePath of the wcp-support-*.tar artifact."""
    data = _fetch_json(build_url + _API_SUFFIX)
    for art in data.get("artifacts", []):
        name = art["fileName"]
        if name.startswith("wcp-support-") and name.endswith(".tar"):
            return art["relativePath"]
    available = [a["fileName"] for a in data.get("artifacts", [])]
    print(f"ERROR: No wcp-support-*.tar in artifacts: {available}", file=sys.stderr)
    raise SystemExit(1)


# ─── Download + extract ────────────────────────────────────────────────────────

def _download(url: str, dest: Path) -> None:
    """Download url to dest using curl with a progress bar."""
    filename = url.rsplit("/", 1)[-1]
    print(f"Downloading {filename} ...", flush=True)
    cmd = [
        *_curl_base(),
        "--progress-bar",    # show % progress bar
        "-o", str(dest),
        url,
    ]
    # Remove -s (silent) so progress bar shows, keep -f (fail on 4xx/5xx)
    cmd = [c for c in cmd if c != "-sf"]
    cmd.insert(1, "-f")
    result = subprocess.run(cmd)
    if result.returncode != 0:
        print(f"\nERROR: curl download failed (exit {result.returncode})", file=sys.stderr)
        raise SystemExit(1)


def _find_master_vm_name(tar_path: Path) -> str:
    """Find the master-vm-*.tgz member name inside the outer tar."""
    with tarfile.open(tar_path, "r:") as t:
        names = [m.name for m in t.getmembers()
                 if m.name.startswith("master-vm-") and m.name.endswith(".tgz")]
    if not names:
        print("ERROR: No master-vm-*.tgz found inside the bundle.", file=sys.stderr)
        print("  Members:", file=sys.stderr)
        with tarfile.open(tar_path, "r:") as t:
            for m in t.getmembers()[:20]:
                print(f"    {m.name}", file=sys.stderr)
        raise SystemExit(1)
    return names[0]


def _extract_master(tar_path: Path, master_name: str, out_dir: Path) -> Path:
    """Extract master-vm-*.tgz from the outer tar into out_dir."""
    out_dir.mkdir(parents=True, exist_ok=True)
    print(f"Extracting {master_name} ...", flush=True)

    with tarfile.open(tar_path, "r:") as outer:
        fobj = outer.extractfile(master_name)
        if fobj is None:
            print(f"ERROR: Cannot extract {master_name}", file=sys.stderr)
            raise SystemExit(1)
        with tarfile.open(fileobj=fobj, mode="r:gz") as inner:
            inner.extractall(out_dir)

    agents = sorted(out_dir.glob("wcp-agent-*"))
    if not agents:
        print(f"ERROR: No wcp-agent-* directory found in {out_dir}", file=sys.stderr)
        raise SystemExit(1)
    return agents[-1]


# ─── Pretty-print env vars ─────────────────────────────────────────────────────

def _first_log(pod_dir: Path, container: str = "manager") -> Path | None:
    """Return the first *.log file in <pod_dir>/<container>/."""
    container_dir = pod_dir / container
    if not container_dir.exists():
        # Try any container dir
        subdirs = [d for d in pod_dir.iterdir() if d.is_dir()]
        if subdirs:
            container_dir = subdirs[0]
        else:
            return None
    logs = sorted(container_dir.glob("*.log"))
    return logs[0] if logs else None


def _print_env(agent_dir: Path) -> None:
    """Print copy-paste shell variables for the triage debugging workflow."""
    pods = agent_dir / "var" / "log" / "pods"
    if not pods.exists():
        print(f"WARNING: {pods} does not exist", file=sys.stderr)
        return

    pod_dirs = sorted(pods.iterdir())

    vmop = next((p for p in pod_dirs if "vmware-system-vmop" in p.name), None)
    nsx  = next((p for p in pod_dirs if "vmware-system-nsx-operator" in p.name), None)
    csi  = next((p for p in pod_dirs if "vmware-system-csi" in p.name), None)

    print("\n" + "─" * 60)
    print("# Paste into your shell to start debugging:\n")
    print(f'SB="{agent_dir}"')

    if vmop:
        log = _first_log(vmop)
        # Decompress any rotated .log.gz files if present
        gz_files = list((vmop / "manager" if (vmop / "manager").exists() else vmop).glob("*.log.*.gz"))
        if gz_files:
            print(f'# Rotated logs found — decompress with:')
            print(f'#   gunzip "{vmop}/manager/"*.log.*.gz 2>/dev/null || true')
        print(f'VMOP_LOG="{log}"' if log else f'VMOP_POD="{vmop}"')
    else:
        print("# WARNING: No vmware-system-vmop pod log found")

    if nsx:
        print(f'NSX_POD="{nsx}"')
    if csi:
        print(f'CSI_POD="{csi}"')

    print()
    print("# ── Suggested searches ───────────────────────────────")
    if vmop and _first_log(vmop):
        print(f'# SNAP: rg "VirtualMachineSnapshot|hasPendingCRVs|snapshot" "$VMOP_LOG" | head -40')
        print(f'# HW:   rg "terminal error|ErrPendingBackfill|UnmanagedVolumes" "$VMOP_LOG" | head -40')
        print(f'# LCM:  rg "OpaqueNetwork|InvalidArgument|deployOVF" "$VMOP_LOG" | head -40')
    if nsx:
        print(f'# NSX:  rg "SubnetPortNotReady|empty NSX resource" "$NSX_POD"/*/0.log | head -20')
    print("─" * 60)


# ─── Main ──────────────────────────────────────────────────────────────────────

def main() -> None:
    ap = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument(
        "build_url",
        help="Jenkins build URL, e.g. https://.../job/prod-vmsvc-reco-vds/1917",
    )
    ap.add_argument(
        "--out",
        default="/tmp/wcp-sb",
        metavar="DIR",
        help="Directory to extract into (default: /tmp/wcp-sb)",
    )
    ap.add_argument(
        "--keep-tar",
        action="store_true",
        help="Keep the downloaded .tar file instead of deleting it after extraction",
    )
    args = ap.parse_args()

    build_url = args.build_url.rstrip("/")
    out_dir   = Path(args.out)

    # 1. Find the artifact
    rel_path     = _find_bundle_artifact(build_url)
    artifact_url = build_url + _ART_SUFFIX.format(rel=rel_path)
    print(f"Found:  {rel_path}")

    # 2. Download to a temp file
    with tempfile.NamedTemporaryFile(suffix=".tar", delete=False,
                                     dir=out_dir.parent if out_dir.parent.exists() else None) as tmp:
        tmp_path = Path(tmp.name)

    try:
        _download(artifact_url, tmp_path)

        # 3. Find master-vm-*.tgz inside
        master_name = _find_master_vm_name(tmp_path)

        # 4. Extract master-vm-*.tgz into out_dir
        agent_dir = _extract_master(tmp_path, master_name, out_dir)

    finally:
        if not args.keep_tar:
            tmp_path.unlink(missing_ok=True)

    print(f"\nExtracted to: {agent_dir}")
    _print_env(agent_dir)


if __name__ == "__main__":
    main()

