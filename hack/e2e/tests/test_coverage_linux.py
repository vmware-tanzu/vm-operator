#!/usr/bin/env python3
# © Broadcom. All Rights Reserved.
# The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

"""Validate Linux coverage IPC in read-only, unprivileged containers.

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
# Connect directly to the real address, bypassing the helper's namespace/UID
# selection, to prove the server rejects unauthorized peers.
REJECTED_PEER = """
import socket, sys
with socket.socket(socket.AF_UNIX) as s:
    s.settimeout(5)
    s.connect(sys.argv[1].replace('@', '\\0', 1))
    try:
        s.sendall(b'GET /snapshot HTTP/1.1\\r\\nHost: localhost\\r\\n\\r\\n')
        assert s.recv(1) == b'', 'unauthorized peer received coverage'
    except (ConnectionResetError, BrokenPipeError):
        pass
"""
SPOOF_SERVER = """
import os, socket
address = '\\0vmoperator-coverage.65534.' + os.readlink('/proc/self/ns/pid')
with socket.socket(socket.AF_UNIX) as s:
    s.bind(address)
    s.listen()
    print('ready', flush=True)
    while True:
        conn, _ = s.accept()
        conn.close()
"""


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
        for name, package, flags in [
            ("server", "./test/coverage/testdata/server",
             ["-cover", "-covermode=atomic", "-coverpkg=" + MODULE + "/test/coverage/testdata/server",
              "-ldflags=-X " + MODULE + "/test/coverage.Revision=" + REVISION]),
            ("coverage-export", "./cmd/coverage-export", []),
        ]:
            subprocess.run(["go", "build", *flags, "-o", str(cls.path / name), package],
                           cwd=ROOT, env=env, check=True)
        (cls.path / "Dockerfile").write_text(
            "FROM python:3.11-alpine\nCOPY server coverage-export /\n")
        cls.image = "vmop-coverage-ipc-test:" + uuid.uuid4().hex
        subprocess.run([cls.cri, "build", "-q", "-t", cls.image, str(cls.path)], check=True)
        cls.addClassCleanup(subprocess.run, [cls.cri, "image", "rm", cls.image],
                            check=True, stdout=subprocess.DEVNULL)

    def start_container(self, command=None, user="65534:65534", network="none"):
        name = "vmop-coverage-ipc-" + uuid.uuid4().hex
        process = subprocess.Popen([
            self.cri, "run", "--rm", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--network=" + network,
            "--user=" + user, "--name", name, self.image, *(command or ["/server"]),
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

        def cleanup():
            subprocess.run([self.cri, "rm", "-f", name], check=False,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            process.communicate(timeout=30)

        self.addCleanup(cleanup)
        self.assertTrue(select.select([process.stdout], [], [], 60)[0],
                        "coverage container did not become ready")
        self.assertTrue(process.stdout.readline(), "coverage container exited before readiness")
        return name

    def export(self, name):
        return subprocess.check_output([self.cri, "exec", name, "/coverage-export"], timeout=30)

    def address(self, name):
        return subprocess.check_output([
            self.cri, "exec", name, "python3", "-c",
            "import os; print('@vmoperator-coverage.65534.' + os.readlink('/proc/self/ns/pid'))",
        ], text=True).strip()

    def test_read_only_container_streams_valid_go_coverage(self):
        name = self.start_container()
        with tempfile.TemporaryDirectory() as output:
            with tarfile.open(fileobj=io.BytesIO(self.export(name)), mode="r:") as archive:
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

    def test_server_rejects_other_uid_in_same_pid_namespace(self):
        name = self.start_container()
        subprocess.run([self.cri, "exec", "--user=0:0", name, "python3", "-c",
                        REJECTED_PEER, self.address(name)], check=True, timeout=30)
        self.assertTrue(self.export(name))

    def test_server_rejects_same_uid_in_different_pid_namespace(self):
        name = self.start_container()
        subprocess.run([
            self.cri, "run", "--rm", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--user=65534:65534",
            "--network=container:" + name, self.image, "python3", "-c",
            REJECTED_PEER, self.address(name),
        ], check=True, timeout=30)
        self.assertTrue(self.export(name))

    def test_helper_rejects_server_with_wrong_uid(self):
        name = self.start_container(["python3", "-u", "-c", SPOOF_SERVER], user="0:0")
        result = subprocess.run([self.cri, "exec", "--user=65534:65534", name, "/coverage-export"],
                                capture_output=True, timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"authenticate coverage manager", result.stderr)

    def test_managers_sharing_network_have_independent_sockets(self):
        first = self.start_container()
        second = self.start_container(network="container:" + first)
        self.assertNotEqual(self.address(first), self.address(second))
        identities = []
        for name in (first, second):
            with tarfile.open(fileobj=io.BytesIO(self.export(name)), mode="r:") as archive:
                identities.append(json.load(archive.extractfile("snapshot.json"))["process_id"])
        self.assertNotEqual(*identities)


if __name__ == "__main__":
    unittest.main()
