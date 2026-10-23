#!/usr/bin/env python3
# © Broadcom. All Rights Reserved.
# The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

"""Deploy, sample, and report operator coverage during E2E runs.

Only setup changes the Deployment. Concurrent runners export cumulative
snapshots without resetting counters. Results describe shared-testbed activity.
"""

import argparse
from collections import deque
import concurrent.futures
import contextlib
import datetime
import http.client
import io
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import threading
import urllib.request


MODULE = "github.com/vmware-tanzu/vm-operator"
RAW_NAME = re.compile(r"(?:covmeta\.[a-f0-9]+|covcounters\.[a-f0-9]+\.\d+\.\d+)")
PROCESS_ID = re.compile(r"[a-f0-9]{32}")
MAX_ARCHIVE_SIZE = 128 * 1024 * 1024
DEFAULT_PORT = 18081


def log(message):
    print(f"[coverage] {message}", file=sys.stderr, flush=True)


def execute(command, timeout=45, **kwargs):
    result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout, check=False, **kwargs)
    if result.returncode:
        raise RuntimeError(f"{command[0]} failed ({result.returncode}): "
                           f"{result.stderr.decode(errors='replace')[-4096:]}")
    return result.stdout


@contextlib.contextmanager
def port_forward(command, remote_port, timeout=20):
    """Yield kubectl's allocated loopback port and always reap the forward."""
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               text=True, bufsize=1)
    ready = threading.Event()
    lines = deque(maxlen=8)
    port = None

    def drain():
        nonlocal port
        try:
            for line in iter(lambda: process.stdout.readline(4096), ""):
                lines.append(line.strip())
                match = re.match(r"Forwarding from 127\.0\.0\.1:(\d+) -> " + str(remote_port) + r"\s*$", line)
                if match:
                    port = int(match[1])
                    ready.set()
        finally:
            ready.set()

    reader = threading.Thread(target=drain, daemon=True)
    try:
        reader.start()
        if not ready.wait(timeout):
            raise RuntimeError("coverage port-forward readiness timed out")
        if port is None or process.poll() is not None:
            raise RuntimeError("coverage port-forward failed: " + "; ".join(lines))
        yield port
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        if reader.ident is not None:
            reader.join(timeout=5)
        process.stdout.close()


def download_snapshot(port):
    """Fetch a bounded snapshot directly from loopback, without HTTP proxies."""
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=30)
    try:
        connection.request("GET", "/snapshot")
        response = connection.getresponse()
        if response.status != 200:
            detail = response.read(4096).decode(errors="replace")
            raise RuntimeError(f"coverage HTTP returned {response.status}: {detail}")
        expected = response.length
        data = response.read(MAX_ARCHIVE_SIZE + 1)
        if len(data) > MAX_ARCHIVE_SIZE:
            raise ValueError("coverage archive exceeds size limit")
        if expected is not None and len(data) != expected:
            raise RuntimeError("coverage HTTP response was truncated")
        return data
    except (OSError, http.client.HTTPException) as err:
        raise RuntimeError(f"download coverage snapshot: {err}") from err
    finally:
        connection.close()


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(value, indent=2) + "\n")
    temporary.replace(path)


def decode_object(data):
    """Reject malformed artifact records before reading their fields."""
    value = json.loads(data)
    if not isinstance(value, dict):
        raise ValueError("expected a JSON object")
    return value


def publish_artifacts(output):
    """Publish the shared artifact path when a pipeline supplies a blob endpoint."""
    url = os.environ.get("E2E_COVERAGE_PUBLISH_URL")
    if not url:
        return
    payload = {"test_fk": os.environ["E2E_COVERAGE_TEST_FK"],
               "deliverable_blob": {"coverage_directory": str(Path(output).resolve())}}
    request = urllib.request.Request(url, data=json.dumps(payload).encode(),
                                     headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        response.read(4096)


def snapshot_order(manifest):
    """Validate and compare UTC snapshot timestamps without losing nanoseconds."""
    stamp = manifest.get("collected_at", "")
    match = re.fullmatch(r"(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z", stamp)
    if not match:
        raise ValueError("invalid snapshot timestamp")
    return (datetime.datetime.strptime(match[1], "%Y-%m-%dT%H:%M:%S"),
            int((match[2] or "0").ljust(9, "0")))


class Collector:
    def __init__(self, output):
        self.output = Path(output).resolve()
        self.source = Path(os.environ.get("E2E_COVERAGE_SOURCE", "/vm-operator/coverage-source")).resolve()
        self.revision = (self.source / "revision").read_text().strip()
        if not re.fullmatch(r"[a-f0-9]{40}", self.revision):
            raise ValueError("coverage source revision must be a full Git commit ID")
        self.namespace = os.environ.get("E2E_COVERAGE_NAMESPACE", "vmware-system-vmop")
        self.deployment = os.environ.get("E2E_COVERAGE_DEPLOYMENT", "vmware-system-vmop-controller-manager")
        self.container = os.environ.get("E2E_COVERAGE_CONTAINER", "manager")
        self.image = os.environ.get("E2E_COVERAGE_IMAGE", "")
        if not self.image:
            raise ValueError("E2E_COVERAGE_IMAGE must identify the tested coverage image")
        self.kubectl = os.environ.get("KUBECTL", "kubectl")
        self.port = int(os.environ.get("E2E_COVERAGE_PORT", str(DEFAULT_PORT)))
        if not 1 <= self.port <= 65535:
            raise ValueError("E2E_COVERAGE_PORT must be between 1 and 65535")
        self.raw = self.output / "raw"
        self.raw.mkdir(parents=True, exist_ok=True)
        self.warnings = []

    def kube(self, *args):
        return execute([self.kubectl, "--request-timeout=30s", "-n", self.namespace, *args])

    def get(self, resource, *args):
        return json.loads(self.kube("get", resource, *args, "-o", "json"))

    def install(self):
        deployment = self.get("deployment", self.deployment)
        spec = deployment["spec"]["template"]["spec"]
        if not any(c["name"] == self.container for c in spec["containers"]):
            raise ValueError(f"Deployment has no {self.container} container")
        patch = {"spec": {"template": {
            "metadata": {"annotations": {"vmoperator.vmware.com/coverage-revision": self.revision}},
            "spec": {
                "containers": [{
                    "name": self.container, "image": self.image, "imagePullPolicy": "IfNotPresent",
                }],
            },
        }}}
        # One pipeline setup job owns installation. Each runner only reads.
        write_json(self.output / "deployment-before.json", deployment)
        self.kube("patch", "deployment", self.deployment, "--type=strategic", "-p", json.dumps(patch))
        execute([self.kubectl, "-n", self.namespace, "rollout", "status",
                 f"deployment/{self.deployment}", "--timeout=10m"], timeout=630)
        errors = self.collect()
        write_json(self.output / "setup-status.json", {"revision": self.revision, "errors": errors})
        if errors:
            raise RuntimeError("; ".join(errors))

    def pods(self):
        deployment = self.get("deployment", self.deployment)
        template = deployment["spec"]["template"]
        container = next(c for c in template["spec"]["containers"] if c["name"] == self.container)
        if container["image"] != self.image:
            raise ValueError("Deployment is not running the requested coverage image")
        uid = deployment["metadata"]["uid"]
        replicasets = self.get("replicasets")["items"]
        owners = {rs["metadata"]["uid"] for rs in replicasets
                  if any(o["uid"] == uid and o.get("controller")
                         for o in rs["metadata"].get("ownerReferences", []))}
        pods = [p for p in self.get("pods")["items"]
                if any(o["uid"] in owners and o.get("controller")
                       for o in p["metadata"].get("ownerReferences", []))]
        return deployment["spec"].get("replicas", 1), pods

    def save(self, archive, pod):
        if len(archive) > MAX_ARCHIVE_SIZE:
            raise ValueError("coverage archive exceeds size limit")
        files = {}
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as reader:
            size = 0
            for member in reader:
                if (not member.isfile() or member.name in files or
                        (member.name != "snapshot.json" and not RAW_NAME.fullmatch(member.name))):
                    raise ValueError(f"invalid coverage archive entry: {member.name}")
                size += member.size
                if size > MAX_ARCHIVE_SIZE:
                    raise ValueError("coverage archive exceeds size limit")
                files[member.name] = reader.extractfile(member).read()
        if ("snapshot.json" not in files or not any(n.startswith("covmeta.") for n in files) or
                not any(n.startswith("covcounters.") for n in files)):
            raise ValueError("coverage archive is missing metadata, counters, or identity")
        manifest = json.loads(files["snapshot.json"])
        if manifest.get("revision") != self.revision:
            raise ValueError("operator coverage revision does not match report source")
        identity = manifest.get("process_id", "")
        if not PROCESS_ID.fullmatch(identity):
            raise ValueError("invalid coverage process identity")
        destination = self.raw / identity
        staging = Path(tempfile.mkdtemp(prefix="snapshot-", dir=self.output))
        try:
            for name, data in files.items():
                (staging / name).write_bytes(data)
            status = next(c for c in pod["status"]["containerStatuses"] if c["name"] == self.container)
            write_json(staging / "pod.json", {
                "name": pod["metadata"]["name"], "uid": pod["metadata"]["uid"],
                "container_id": status.get("containerID"), "image_id": status.get("imageID"),
                "restart_count": status.get("restartCount", 0),
            })
            # One writer per artifact directory. Only the latest cumulative
            # snapshot per process is merged; old process identities survive.
            if destination.exists():
                shutil.rmtree(destination)
            staging.rename(destination)
        finally:
            if staging.exists():
                shutil.rmtree(staging)

    def sample(self, pod):
        name = pod["metadata"]["name"]
        statuses = {c["name"]: c for c in pod.get("status", {}).get("containerStatuses", [])}
        status = statuses.get(self.container, {})
        if "running" not in status.get("state", {}):
            raise RuntimeError(f"{name}: manager is not running")
        spec = next(c for c in pod["spec"]["containers"] if c["name"] == self.container)
        if spec["image"] != self.image:
            raise RuntimeError(f"{name}: unexpected manager image")
        command = [self.kubectl, "-n", self.namespace, "port-forward", "--address=127.0.0.1",
                   "--pod-running-timeout=20s", f"pod/{name}", f":{self.port}"]
        with port_forward(command, self.port) as local_port:
            archive = download_snapshot(local_port)
        self.save(archive, pod)

    def collect(self):
        errors = []
        try:
            desired, pods = self.pods()
            active = [p for p in pods if not p["metadata"].get("deletionTimestamp")]
            if desired < 1 or len(active) < desired:
                errors.append(f"expected {desired} active replicas, found {len(active)}")
            if not pods:
                errors.append("no operator replicas found")
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                futures = {pool.submit(self.sample, p): p["metadata"]["name"] for p in pods}
                for future, name in futures.items():
                    try:
                        future.result()
                    except (OSError, ValueError, RuntimeError, KeyError, StopIteration, tarfile.TarError,
                            subprocess.SubprocessError) as err:
                        errors.append(f"{name}: {err}")
        except (OSError, ValueError, RuntimeError, KeyError, StopIteration, subprocess.SubprocessError) as err:
            errors.append(str(err))
        for error in errors:
            log(error)
        return errors

    def archive_raw(self):
        """Preserve validated raw data without running the report tools."""
        directories = sorted(p for p in self.raw.iterdir() if p.is_dir())
        if not directories:
            raise ValueError("no coverage snapshots collected")
        for directory in directories:
            manifest = json.loads((directory / "snapshot.json").read_text())
            if manifest.get("revision") != self.revision:
                raise ValueError("stored snapshot revision does not match report source")
        # Preserve raw evidence even if a reporting tool later fails.
        with tarfile.open(self.output / "coverage-raw.tar.gz", "w:gz") as archive:
            archive.add(self.raw, arcname="raw")
        return directories

    def report(self):
        directories = self.archive_raw()
        merged = self.output / "merged"
        merged.mkdir(exist_ok=True)
        covdata = os.environ.get("E2E_COVDATA_TOOL", "covdata")
        cover = os.environ.get("E2E_COVER_TOOL", "cover")
        execute([covdata, "merge", "-i=" + ",".join(str(p) for p in directories), f"-o={merged}"], timeout=120)
        profile = self.output / "coverage.out"
        execute([covdata, "textfmt", f"-i={merged}", f"-o={profile}"], timeout=120)
        lines = profile.read_text().splitlines()
        if len(lines) < 2 or lines[0] != "mode: atomic":
            raise ValueError("empty or non-atomic coverage profile")
        # Absolute source names let the standalone cover tool work offline,
        # without a Go installation/module cache or paths from the build host.
        mapped = [lines[0]]
        for line in lines[1:]:
            filename, block = line.rsplit(":", 1)
            if not filename.startswith(MODULE + "/"):
                raise ValueError(f"unexpected coverage source: {filename}")
            source = (self.source / filename[len(MODULE) + 1:]).resolve()
            if self.source not in source.parents or not source.is_file():
                raise ValueError(f"coverage source is unavailable: {filename}")
            mapped.append(f"{source}:{block}")
        local = self.output / "coverage-local.out"
        local.write_text("\n".join(mapped) + "\n")
        summary = execute([cover, f"-func={local}"], timeout=120).decode()
        summary = summary.replace(str(self.source), MODULE)
        (self.output / "coverage-summary.txt").write_text(summary)
        execute([cover, f"-html={local}", f"-o={self.output / 'coverage.html'}"], timeout=120)
        log(summary.splitlines()[-1])


def run_suite(collector, command):
    if any(collector.raw.iterdir()):
        raise ValueError("coverage output contains an earlier run; use a fresh artifact directory")
    interval = float(os.environ.get("E2E_COVERAGE_INTERVAL", "60"))
    if interval <= 0:
        raise ValueError("E2E_COVERAGE_INTERVAL must be positive")
    stop = threading.Event()
    errors = collector.collect()
    test_exit = None
    if not errors:
        def poll():
            while not stop.wait(interval):
                collector.warnings.extend(collector.collect())

        worker = threading.Thread(target=poll, daemon=True)
        worker.start()
        process = None
        previous = {}
        try:
            process = subprocess.Popen(command, start_new_session=True)

            def forward(signum, _frame):
                if process.poll() is None:
                    os.killpg(process.pid, signum)

            for signum in (signal.SIGTERM, signal.SIGINT):
                previous[signum] = signal.signal(signum, forward)
            test_exit = process.wait()
            if test_exit < 0:
                test_exit = 128 - test_exit
        finally:
            stop.set()
            worker.join()
            for signum, handler in previous.items():
                signal.signal(signum, handler)
        errors.extend(collector.collect())
    report_enabled = os.environ.get("E2E_COVERAGE_REPORT", "true") == "true"
    report_generated = False
    try:
        if report_enabled:
            collector.report()
            report_generated = True
        else:
            collector.archive_raw()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as err:
        errors.append(f"report failed: {err}")
    write_json(collector.output / "status.json", {
        "revision": collector.revision, "image": collector.image,
        "test_exit_code": test_exit, "report_generated": report_generated,
        "coverage_passed": not errors, "errors": errors,
        "sampling_warnings": collector.warnings,
        "scope": "cumulative shared-Supervisor activity; includes setup and concurrent suites",
        "limitation": "a killed process can lose activity since its last successful snapshot",
    })
    try:
        publish_artifacts(collector.output)
    except (OSError, ValueError, KeyError) as err:
        errors.append(f"artifact publication failed: {err}")
        status = json.loads((collector.output / "status.json").read_text())
        status.update(coverage_passed=False, errors=errors)
        write_json(collector.output / "status.json", status)
    return test_exit if test_exit else (2 if errors else 0)


def aggregate(collector, inputs):
    """Merge each process's latest snapshot across all expected suite artifacts.

    Inputs map suite names to artifact directories (local runs) or HTTP blob
    URLs (pipelines). Pipeline manifests point into the shared log mount.
    A failed test can still contribute complete coverage. Missing/failed
    collection makes the combined report incomplete and returns failure.
    """
    if any(collector.raw.iterdir()):
        raise ValueError("aggregate output contains an earlier run; use a fresh artifact directory")
    if not isinstance(inputs, dict) or not inputs:
        raise ValueError("aggregate requires at least one named suite input")
    errors, suites, selected = [], {}, {}
    for name, location in sorted(inputs.items()):
        try:
            if not isinstance(location, str) or not location:
                raise ValueError("missing artifact location")
            if location.startswith(("http://", "https://")):
                with urllib.request.urlopen(location, timeout=30) as response:
                    manifest = decode_object(response.read(1024 * 1024))
                # Test-blob APIs may wrap the posted deliverable in their record.
                manifest = manifest.get("deliverable_blob", manifest)
                if not isinstance(manifest, dict):
                    raise ValueError("expected a manifest object")
                directory = Path(manifest["coverage_directory"]).resolve()
                log_root = Path(os.environ.get("E2E_COVERAGE_LOG_ROOT", "/cpbu/logs")).resolve()
                if log_root not in directory.parents:
                    raise ValueError("artifact directory is outside the shared log mount")
            else:
                directory = Path(location).resolve()
            status = decode_object((directory / "status.json").read_text())
            if status.get("revision") != collector.revision or status.get("image") != collector.image:
                raise ValueError("suite revision/image does not match the coverage build")
            suites[name] = {"directory": str(directory), "test_exit_code": status.get("test_exit_code"),
                            "coverage_passed": status.get("coverage_passed", False),
                            "sampling_warnings": status.get("sampling_warnings", [])}
            if not status.get("coverage_passed"):
                errors.append(f"{name}: coverage collection failed: {status.get('errors', [])}")
            snapshots = sorted((directory / "raw").iterdir())
            if not snapshots:
                raise ValueError("no coverage snapshots collected")
            for snapshot in snapshots:
                if snapshot.is_symlink() or not snapshot.is_dir() or not PROCESS_ID.fullmatch(snapshot.name):
                    raise ValueError("invalid snapshot directory")
                manifest = decode_object((snapshot / "snapshot.json").read_text())
                if manifest.get("revision") != collector.revision or manifest.get("process_id") != snapshot.name:
                    raise ValueError("snapshot revision/process identity mismatch")
                files = list(snapshot.iterdir())
                if any(p.is_symlink() or not p.is_file() or
                       (p.name not in ("snapshot.json", "pod.json") and not RAW_NAME.fullmatch(p.name))
                       for p in files):
                    raise ValueError("invalid snapshot file")
                metadata = sorted(p.name for p in files if p.name.startswith("covmeta."))
                if not metadata or not any(p.name.startswith("covcounters.") for p in files):
                    raise ValueError("snapshot is missing metadata or counters")
                pod = decode_object((snapshot / "pod.json").read_text())
                identity = (manifest.get("go_version"), pod.get("uid"),
                            pod.get("container_id"), pod.get("image_id"), metadata)
                order = snapshot_order(manifest)
                prior = selected.get(snapshot.name)
                if prior and prior[2] != identity:
                    raise ValueError("conflicting identity for a shared process snapshot")
                if not prior or order > prior[0]:
                    selected[snapshot.name] = (order, snapshot, identity)
        except (OSError, ValueError, KeyError, TypeError) as err:
            errors.append(f"{name}: {err}")
            suites.setdefault(name, {"coverage_passed": False})
    for identity, (_, source, _) in selected.items():
        shutil.copytree(source, collector.raw / identity)
    report_generated = False
    try:
        collector.report()
        report_generated = True
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as err:
        errors.append(f"aggregate report failed: {err}")
    write_json(collector.output / "status.json", {
        "revision": collector.revision, "image": collector.image,
        "coverage_passed": not errors, "errors": errors,
        "report_generated": report_generated, "suites": suites,
        "expected_suites": len(inputs), "processes": len(selected),
        "scope": "cumulative coverage across the selected testbeds; latest snapshot per process",
        "limitation": "a killed process can lose activity since its last successful snapshot",
    })
    return 2 if errors else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("setup", "collect", "report", "run", "aggregate"))
    parser.add_argument("--output", required=True)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    try:
        collector = Collector(args.output)
        if args.action == "setup":
            collector.install()
        elif args.action == "collect":
            errors = collector.collect()
            if errors:
                raise RuntimeError("; ".join(errors))
        elif args.action == "report":
            collector.report()
        elif args.action == "aggregate":
            return aggregate(collector, json.loads(os.environ.get("E2E_COVERAGE_INPUTS", "{}")))
        else:
            command = args.command[1:] if args.command[:1] == ["--"] else args.command
            if not command:
                raise ValueError("run requires a test command after --")
            return run_suite(collector, command)
        return 0
    except (OSError, ValueError, RuntimeError, KeyError, StopIteration, subprocess.SubprocessError) as err:
        log(str(err))
        write_json(Path(args.output) / "status.json", {
            "coverage_passed": False, "test_exit_code": None, "errors": [str(err)],
        })
        return 2


if __name__ == "__main__":
    sys.exit(main())
