#!/usr/bin/env python3
# © Broadcom. All Rights Reserved.
# The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

"""Validate coverage HTTP in read-only, unprivileged Linux containers.

Run explicitly with a local Docker-compatible runtime:
  python3 -m unittest discover -s hack/e2e/tests -p test_coverage_linux.py -v
COVERAGE_TEST_CRI may name podman instead of docker.
"""

import io
import json
import os
from pathlib import Path
import select
import subprocess
import tarfile
import tempfile
import unittest
import uuid


ROOT = Path(__file__).resolve().parents[3]
MODULE = "github.com/vmware-tanzu/vm-operator"
REVISION = "a" * 40
FETCH = ("import sys, urllib.request; "
         "sys.stdout.buffer.write(urllib.request.urlopen("
         "'http://127.0.0.1:' + sys.argv[1] + '/snapshot', timeout=10).read())")


class LinuxCoverageTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cri = os.environ.get("COVERAGE_TEST_CRI", "docker")
        cls.tmp = tempfile.TemporaryDirectory(prefix="vmop-coverage-linux-")
        cls.addClassCleanup(cls.tmp.cleanup)
        cls.path = Path(cls.tmp.name)
        arch = subprocess.check_output(
            [cls.cri, "info", "--format", "{{.Architecture}}" if cls.cri.endswith("docker")
             else "{{.Host.Arch}}"], text=True).strip()
        arch = {"aarch64": "arm64", "x86_64": "amd64"}.get(arch, arch)
        env = dict(os.environ, GOOS="linux", GOARCH=arch, CGO_ENABLED="0")
        subprocess.run([
            "go", "build", "-cover", "-covermode=atomic",
            "-coverpkg=" + MODULE + "/test/coverage/testdata/server",
            "-ldflags=-X " + MODULE + "/test/coverage.Revision=" + REVISION,
            "-o", str(cls.path / "server"), "./test/coverage/testdata/server",
        ], cwd=ROOT, env=env, check=True)
        (cls.path / "Dockerfile").write_text("FROM python:3.11-alpine\nCOPY server /server\n")
        cls.image = "vmop-coverage-http-test:" + uuid.uuid4().hex
        subprocess.run([cls.cri, "build", "-q", "-t", cls.image, str(cls.path)], check=True)
        cls.addClassCleanup(subprocess.run, [cls.cri, "image", "rm", cls.image],
                            check=True, stdout=subprocess.DEVNULL)

    def start_container(self, port=0, network="none"):
        name = "vmop-coverage-http-" + uuid.uuid4().hex
        process = subprocess.Popen([
            self.cri, "run", "--rm", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--network=" + network,
            "--user=65534:65534", "--name", name, self.image, "/server", "-port", str(port),
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

        def cleanup():
            subprocess.run([self.cri, "rm", "-f", name], check=False,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            process.communicate(timeout=30)

        self.addCleanup(cleanup)
        self.assertTrue(select.select([process.stdout], [], [], 60)[0],
                        "coverage container did not become ready")
        ready = process.stdout.readline()
        self.assertTrue(ready, "coverage container exited before readiness")
        address = ready.decode().split()[0]
        self.assertTrue(address.startswith("127.0.0.1:"))
        return name, int(address.rsplit(":", 1)[1])

    def export(self, name, port):
        return subprocess.check_output(
            [self.cri, "exec", name, "python3", "-c", FETCH, str(port)], timeout=30)

    def test_read_only_container_streams_valid_go_coverage(self):
        name, port = self.start_container()
        with tempfile.TemporaryDirectory() as output:
            with tarfile.open(fileobj=io.BytesIO(self.export(name, port)), mode="r:") as archive:
                self.assertEqual(len(archive.getnames()), 3)
                for entry in archive:
                    self.assertEqual(Path(entry.name).name, entry.name)
                    (Path(output) / entry.name).write_bytes(archive.extractfile(entry).read())
            manifest = json.loads((Path(output) / "snapshot.json").read_text())
            self.assertEqual(manifest["revision"], REVISION)
            result = subprocess.check_output(["go", "tool", "covdata", "percent", "-i=" + output],
                                             cwd=ROOT, text=True)
            self.assertIn("coverage:", result)
        diff = subprocess.check_output([self.cri, "diff", name], text=True).strip()
        self.assertEqual(diff, "", "coverage wrote to the container filesystem")

    def test_server_is_not_reachable_on_non_loopback_interface(self):
        name, port = self.start_container(network="bridge")
        check = (
            "import socket, sys; "
            "address = socket.gethostbyname(socket.gethostname()); "
            "assert not address.startswith('127.'); "
            "s = socket.socket(); s.settimeout(2); "
            "assert s.connect_ex((address, int(sys.argv[1]))) != 0; s.close()"
        )
        subprocess.run([self.cri, "exec", name, "python3", "-c", check, str(port)],
                       check=True, timeout=30)
        self.assertTrue(self.export(name, port))

    def test_node_local_endpoint_is_deliberately_unauthenticated(self):
        name, port = self.start_container()
        snapshot = subprocess.check_output([
            self.cri, "run", "--rm", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--user=0:0",
            "--network=container:" + name, self.image, "python3", "-c", FETCH, str(port),
        ], timeout=30)
        with tarfile.open(fileobj=io.BytesIO(snapshot), mode="r:") as archive:
            self.assertIn("snapshot.json", archive.getnames())

    def test_port_collision_fails_without_displacing_existing_manager(self):
        name, port = self.start_container(port=18081)
        result = subprocess.run([
            self.cri, "run", "--rm", "--read-only", "--cap-drop=ALL",
            "--network=container:" + name, "--user=65534:65534",
            self.image, "/server", "-port", str(port),
        ], capture_output=True, timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"listen on coverage TCP port", result.stderr)
        self.assertTrue(self.export(name, port))

    def test_managers_sharing_network_can_use_distinct_ports(self):
        first, port1 = self.start_container(port=18081)
        second, port2 = self.start_container(port=18082, network="container:" + first)
        identities = []
        for name, port in ((first, port1), (second, port2)):
            with tarfile.open(fileobj=io.BytesIO(self.export(name, port)), mode="r:") as archive:
                identities.append(json.load(archive.extractfile("snapshot.json"))["process_id"])
        self.assertNotEqual(*identities)


if __name__ == "__main__":
    unittest.main()
