#!/usr/bin/env python3
# © Broadcom. All Rights Reserved.
# The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

"""Exercise real Go coverage exports and the runner without a Supervisor."""

import importlib.util
import concurrent.futures
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[3]
SPEC = importlib.util.spec_from_file_location("vmop_coverage", ROOT / "hack/e2e/coverage.py")
coverage = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(coverage)
REVISION = "a" * 40
IMAGE = "example.test/vmoperator-coverage:123"


class CoverageTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix="vmop-cov-build-", dir="/private/tmp" if sys.platform == "darwin" else None)
        cls.binaries = Path(cls.build.name)
        env = dict(os.environ, GOCACHE=os.environ.get("GOCACHE", str(cls.binaries / "cache")))
        commands = [
            ["go", "build", "-cover", "-covermode=atomic", "-tags=e2e_coverage",
             "-o", str(cls.binaries / "manager"), "."],
            ["go", "build", "-cover", "-covermode=atomic",
             "-coverpkg=" + coverage.MODULE + "/test/coverage/testdata/server",
             "-ldflags=-X " + coverage.MODULE + "/test/coverage.Revision=" + REVISION,
             "-o", str(cls.binaries / "server"), "./test/coverage/testdata/server"],
            ["go", "build", "-o", str(cls.binaries / "export"), "./cmd/coverage-export"],
            ["go", "build", "-o", str(cls.binaries / "covdata"), "cmd/covdata"],
            ["go", "build", "-o", str(cls.binaries / "cover"), "cmd/cover"],
        ]
        for command in commands:
            subprocess.run(command, cwd=ROOT, env=env, check=True, capture_output=True)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="vmop-cov-", dir="/private/tmp" if sys.platform == "darwin" else None)
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)
        self.source = self.path / "source"
        fixture = Path("test/coverage/testdata/server/main.go")
        (self.source / fixture).parent.mkdir(parents=True)
        shutil.copy(ROOT / fixture, self.source / fixture)
        (self.source / "revision").write_text(REVISION)
        self.env = mock.patch.dict(os.environ, {
            "E2E_COVERAGE_SOURCE": str(self.source), "E2E_COVERAGE_IMAGE": IMAGE,
            "E2E_COVDATA_TOOL": str(self.binaries / "covdata"),
            "E2E_COVER_TOOL": str(self.binaries / "cover"),
            "TMPDIR": str(self.path), "E2E_COVERAGE_INTERVAL": "60",
        })
        self.env.start()
        self.addCleanup(self.env.stop)
        os.environ.pop("GOCOVERDIR", None)
        self.pod = {
            "metadata": {"name": "manager-1", "uid": "pod-1",
                         "ownerReferences": [{"uid": "rs-1", "controller": True}]},
            "spec": {"containers": [{"name": "manager", "image": IMAGE}]},
            "status": {"containerStatuses": [{
                "name": "manager", "containerID": "containerd://one", "imageID": IMAGE,
                "restartCount": 0, "state": {"running": {}},
            }]},
        }
        self.server = self.start_server()
        self.collector = coverage.Collector(self.path / "results")

    def start_server(self):
        process = subprocess.Popen([str(self.binaries / "server")], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

        def cleanup():
            if process.poll() is None:
                process.terminate()
            process.communicate(timeout=10)

        self.addCleanup(cleanup)
        # The fixture writes a line only after the private socket is listening.
        ready = process.stdout.readline()
        if not ready:
            self.fail(process.stderr.read().decode())
        return process

    def snapshot(self):
        return subprocess.check_output([str(self.binaries / "export")])

    def configure_kubectl(self, missing=False):
        state = {
            "deployment": {"metadata": {"uid": "deployment-1"}, "spec": {
                "replicas": 2 if missing else 1, "template": self.pod}},
            "replicasets": {"items": [{"metadata": {"uid": "rs-1", "ownerReferences": [
                {"uid": "deployment-1", "controller": True}]}}]},
            "pods": {"items": [self.pod]},
        }
        state_path = self.path / "kube.json"
        state_path.write_text(json.dumps(state))
        kubectl = self.path / "kubectl"
        kubectl.write_text(
            "#!" + sys.executable + "\n"
            "import json, subprocess, sys\n"
            f"state = json.load(open({str(state_path)!r}))\n"
            "if 'get' in sys.argv:\n"
            "    print(json.dumps(state[sys.argv[sys.argv.index('get') + 1]]))\n"
            "elif 'exec' in sys.argv:\n"
            f"    sys.exit(subprocess.call([{str(self.binaries / 'export')!r}]))\n"
            "else:\n"
            "    sys.exit(99)\n"
        )
        kubectl.chmod(0o755)
        os.environ["KUBECTL"] = str(kubectl)
        self.collector.kubectl = str(kubectl)

    def test_real_snapshots_replace_cumulative_counters_and_report_offline(self):
        self.collector.save(self.snapshot(), self.pod)
        self.collector.save(self.snapshot(), self.pod)
        self.assertEqual(len(list(self.collector.raw.iterdir())), 1)
        self.assertEqual(len(list(self.collector.raw.glob("*/covcounters.*"))), 1)
        self.collector.report()
        summary = (self.collector.output / "coverage-summary.txt").read_text()
        self.assertIn("exercise", summary)
        self.assertIn("66.7%", summary)
        self.assertIn("total:", summary)
        self.assertTrue((self.collector.output / "coverage.html").is_file())
        self.assertTrue((self.collector.output / "coverage-raw.tar.gz").is_file())

    def test_actual_manager_initializes_export_after_go_coverage_metadata(self):
        self.server.terminate()
        self.server.communicate(timeout=10)
        result = subprocess.run([str(self.binaries / "manager"), "--help"],
                                capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertFalse(list(self.path.rglob("covmeta.*")))
        self.assertFalse(list(self.path.rglob("covcounters.*")))

    def test_streamed_snapshots_need_no_coverage_directory(self):
        self.assertNotIn("GOCOVERDIR", os.environ)
        before = set(self.path.rglob("*"))
        with tarfile.open(fileobj=io.BytesIO(self.snapshot()), mode="r:") as archive:
            self.assertEqual(len(archive.getnames()), 3)
            meta = next(n for n in archive.getnames() if n.startswith("covmeta."))
            digest = archive.extractfile(meta).read()[24:40].hex()
            self.assertEqual(meta, "covmeta." + digest)
            self.assertTrue(any(n.startswith("covcounters." + digest + ".")
                                for n in archive.getnames()))
        self.assertEqual(set(self.path.rglob("*")), before)

    def test_concurrent_exports_remain_valid_cumulative_snapshots(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            snapshots = list(executor.map(lambda _: self.snapshot(), range(8)))
        for snapshot in snapshots:
            self.collector.save(snapshot, self.pod)
        self.assertEqual(len(list(self.collector.raw.iterdir())), 1)
        self.collector.report()

    def test_second_manager_cannot_displace_live_exporter(self):
        result = subprocess.run([str(self.binaries / "server")],
                                capture_output=True, timeout=10)
        self.assertNotEqual(result.returncode, 0)
        self.collector.save(self.snapshot(), self.pod)

    def test_hard_kill_allows_restart_and_preserves_exported_data(self):
        self.collector.save(self.snapshot(), self.pod)
        self.server.kill()
        self.server.communicate(timeout=10)
        self.server = self.start_server()
        self.collector.save(self.snapshot(), self.pod)
        self.assertEqual(len(list(self.collector.raw.iterdir())), 2)
        self.collector.report()

    def test_restart_retains_previous_process_snapshot(self):
        self.collector.save(self.snapshot(), self.pod)
        self.server.terminate()
        self.server.communicate(timeout=10)
        self.server = self.start_server()
        self.collector.save(self.snapshot(), self.pod)
        self.assertEqual(len(list(self.collector.raw.iterdir())), 2)
        self.collector.report()

    def test_wrong_revision_is_rejected(self):
        self.collector.revision = "b" * 40
        with self.assertRaisesRegex(ValueError, "revision"):
            self.collector.save(self.snapshot(), self.pod)
        self.assertEqual(list(self.collector.raw.iterdir()), [])

    def test_archive_traversal_and_missing_counters_are_rejected(self):
        for name in ("../escape", "snapshot.json"):
            with self.subTest(name=name):
                data = io.BytesIO()
                with tarfile.open(fileobj=data, mode="w") as archive:
                    entry = tarfile.TarInfo(name)
                    entry.size = 2
                    archive.addfile(entry, io.BytesIO(b"{}"))
                with self.assertRaises(ValueError):
                    self.collector.save(data.getvalue(), self.pod)
        self.assertFalse((self.path / "escape").exists())

    def test_empty_collection_cannot_report_success(self):
        with self.assertRaisesRegex(ValueError, "no coverage"):
            self.collector.report()

    def test_missing_replica_is_visible(self):
        self.configure_kubectl(missing=True)
        self.assertIn("expected 2 active replicas", "; ".join(self.collector.collect()))

    def test_all_owned_replicas_are_sampled_including_nonleaders(self):
        self.configure_kubectl()
        state_path = self.path / "kube.json"
        state = json.loads(state_path.read_text())
        state["deployment"]["spec"]["replicas"] = 2
        second = json.loads(json.dumps(self.pod))
        second["metadata"].update(name="manager-2", uid="pod-2")
        unrelated = json.loads(json.dumps(self.pod))
        unrelated["metadata"].update(name="other-app", ownerReferences=[])
        state["pods"]["items"].extend([second, unrelated])
        state_path.write_text(json.dumps(state))
        with mock.patch.object(self.collector, "sample") as sample:
            self.assertEqual(self.collector.collect(), [])
        self.assertEqual({c.args[0]["metadata"]["name"] for c in sample.call_args_list},
                         {"manager-1", "manager-2"})

    def test_setup_changes_only_the_manager_and_waits_for_rollout(self):
        self.configure_kubectl()
        deployment = self.collector.get("deployment", self.collector.deployment)
        deployment["spec"]["template"]["spec"]["securityContext"] = {"fsGroup": 1234}
        with mock.patch.object(self.collector, "get", return_value=deployment), \
                mock.patch.object(self.collector, "kube") as kube, \
                mock.patch.object(self.collector, "collect", return_value=[]), \
                mock.patch.object(coverage, "execute") as execute:
            self.collector.install()
        args = kube.call_args.args
        self.assertEqual(args[:3], ("patch", "deployment", self.collector.deployment))
        patch = json.loads(args[-1])["spec"]["template"]["spec"]
        self.assertEqual(patch, {"containers": [{"name": "manager", "image": IMAGE,
                                               "imagePullPolicy": "IfNotPresent"}]})
        self.assertIn("rollout", execute.call_args.args[0])

    def test_old_run_and_old_revision_cannot_contaminate_reports(self):
        self.collector.save(self.snapshot(), self.pod)
        with self.assertRaisesRegex(ValueError, "earlier run"):
            coverage.run_suite(self.collector, [sys.executable, "-c", "pass"])
        self.collector.revision = "b" * 40
        with self.assertRaisesRegex(ValueError, "stored snapshot revision"):
            self.collector.report()

    def test_failed_test_keeps_exit_code_and_still_reports(self):
        self.configure_kubectl()
        code = coverage.run_suite(self.collector, [sys.executable, "-c", "raise SystemExit(7)"])
        self.assertEqual(code, 7)
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertEqual(status["test_exit_code"], 7)
        self.assertTrue(status["coverage_passed"])
        self.assertTrue((self.collector.output / "coverage.html").exists())

    def test_report_failure_fails_successful_suite(self):
        self.configure_kubectl()
        with mock.patch.dict(os.environ, {"E2E_COVER_TOOL": "/nonexistent-cover"}):
            code = coverage.run_suite(self.collector, [sys.executable, "-c", "pass"])
        self.assertEqual(code, 2)
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertEqual(status["test_exit_code"], 0)
        self.assertFalse(status["coverage_passed"])
        self.assertTrue((self.collector.output / "coverage-raw.tar.gz").exists())

    def test_missing_coverage_prevents_test_execution(self):
        self.configure_kubectl(missing=True)
        marker = self.path / "test-ran"
        code = coverage.run_suite(self.collector, [sys.executable, "-c",
                                                   f"open({str(marker)!r}, 'w').close()"])
        self.assertEqual(code, 2)
        self.assertFalse(marker.exists())
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertTrue(status["report_generated"])
        self.assertFalse(status["coverage_passed"])

    def test_runner_callback_waits_for_coverage_and_preserves_failure(self):
        self.check_runner_callback(report=True)

    def test_collection_only_callback_waits_for_raw_artifacts_and_preserves_failure(self):
        self.check_runner_callback(report=False)

    def test_standard_code_coverage_flag_activates_collection_and_callback_status(self):
        self.check_runner_callback(report=False, settings={"CODE_COVERAGE": "enabled"})

    def test_ordinary_runner_does_not_collect_coverage(self):
        self.check_runner_callback(report=False, settings={}, enabled=False)

    def check_runner_callback(self, report, settings=None, enabled=True):
        self.configure_kubectl()
        artifacts = self.path / "artifacts"
        artifacts.mkdir()
        test = self.path / "test"
        test.write_text("#!/bin/sh\nexit 7\n")
        test.chmod(0o755)
        curl = self.path / "curl"
        artifact_check = (
            f"assert (p / 'coverage/{'coverage.html' if report else 'coverage-raw.tar.gz'}').exists()\n"
            "assert (p / 'coverage/status.json').exists()\n"
        ) if enabled else "assert not (p / 'coverage').exists()\n"
        curl.write_text(
            "#!" + sys.executable + "\n"
            "import json, pathlib, sys\n"
            f"p = pathlib.Path({str(artifacts)!r})\n"
            + artifact_check +
            "(p / 'callback.json').write_text(sys.argv[sys.argv.index('-d') + 1])\n"
        )
        curl.chmod(0o755)
        env = dict(os.environ, E2E_ARTIFACT_FOLDER=str(artifacts),
                   E2E_COVERAGE_REPORT="true" if report else "false",
                   E2E_PREBUILT_BINARY=str(test), TEST_CALLBACK_URL="http://unused",
                   PATH=str(self.path) + os.pathsep + os.environ["PATH"])
        env.pop("CODE_COVERAGE", None)
        env.pop("E2E_COVERAGE", None)
        env.update({"E2E_COVERAGE": "true"} if settings is None else settings)
        result = subprocess.run(["bash", str(ROOT / "hack/e2e/run-e2e.sh"), "prebuilt"],
                                cwd=ROOT, env=env, capture_output=True, timeout=90)
        self.assertEqual(result.returncode, 7, result.stderr.decode())
        callback = json.loads((artifacts / "callback.json").read_text())
        self.assertEqual(callback["result"], "Failed")
        if enabled:
            self.assertEqual(callback["subtest_details"][-1]["result"], "PASS")
        else:
            self.assertEqual(callback["subtest_details"], [])

    def save_suite(self, name, test_exit=0, passed=True):
        collector = coverage.Collector(self.path / name)
        collector.save(self.snapshot(), self.pod)
        coverage.write_json(collector.output / "status.json", {
            "revision": REVISION, "image": IMAGE, "test_exit_code": test_exit,
            "coverage_passed": passed, "errors": [] if passed else ["replica missing"],
        })
        return collector

    def test_collection_only_does_not_invoke_report_tools_and_still_publishes(self):
        self.configure_kubectl()
        with mock.patch.dict(os.environ, {"E2E_COVERAGE_REPORT": "false",
                                          "E2E_COVDATA_TOOL": "/missing-covdata",
                                          "E2E_COVER_TOOL": "/missing-cover"}), \
                mock.patch.object(coverage, "publish_artifacts") as publish:
            code = coverage.run_suite(self.collector, [sys.executable, "-c", "raise SystemExit(7)"])
        self.assertEqual(code, 7)
        self.assertTrue((self.collector.output / "coverage-raw.tar.gz").is_file())
        self.assertFalse((self.collector.output / "coverage.html").exists())
        self.assertTrue(json.loads((self.collector.output / "status.json").read_text())["coverage_passed"])
        publish.assert_called_once_with(self.collector.output)

    def test_publication_failure_is_visible_before_callback(self):
        self.configure_kubectl()
        with mock.patch.dict(os.environ, {"E2E_COVERAGE_REPORT": "false"}), \
                mock.patch.object(coverage, "publish_artifacts", side_effect=OSError("HTTP unavailable")):
            code = coverage.run_suite(self.collector, [sys.executable, "-c", "pass"])
        self.assertEqual(code, 2)
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertFalse(status["coverage_passed"])
        self.assertIn("publication failed", status["errors"][-1])

    def test_aggregate_uses_latest_shared_process_once_and_accepts_failed_tests(self):
        older = self.save_suite("older")
        newer = self.save_suite("newer", test_exit=7)
        old_manifest = next(older.raw.glob("*/snapshot.json"))
        data = json.loads(old_manifest.read_text())
        data["collected_at"] = "2020-01-01T00:00:00Z"
        coverage.write_json(old_manifest, data)
        newer.report()
        code = coverage.aggregate(self.collector, {"a-newer": str(newer.output), "z-older": str(older.output)})
        self.assertEqual(code, 0)
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertEqual(status["processes"], 1)
        self.assertEqual(status["suites"]["a-newer"]["test_exit_code"], 7)
        self.assertEqual(next(self.collector.raw.glob("*/snapshot.json")).read_bytes(),
                         next(newer.raw.glob("*/snapshot.json")).read_bytes())
        self.assertEqual((self.collector.output / "coverage.out").read_text(),
                         (newer.output / "coverage.out").read_text())

    def test_aggregate_retains_replaced_processes(self):
        first = self.save_suite("first")
        self.server.terminate()
        self.server.communicate(timeout=10)
        self.server = self.start_server()
        second = self.save_suite("second")
        self.assertEqual(coverage.aggregate(self.collector, {
            "first": str(first.output), "second": str(second.output)}), 0)
        self.assertEqual(len(list(self.collector.raw.iterdir())), 2)

    def test_aggregate_reports_partial_data_and_fails_missing_or_failed_collections(self):
        first = self.save_suite("first", passed=False)
        code = coverage.aggregate(self.collector, {"first": str(first.output),
                                                  "missing": str(self.path / "missing")})
        self.assertEqual(code, 2)
        status = json.loads((self.collector.output / "status.json").read_text())
        self.assertTrue(status["report_generated"])
        self.assertFalse(status["coverage_passed"])
        self.assertEqual(len(status["errors"]), 2)

    def test_aggregate_rejects_wrong_build_timestamp_and_symlink(self):
        suite = self.save_suite("suite")
        status_file = suite.output / "status.json"
        status = json.loads(status_file.read_text())
        snapshot = next(suite.raw.iterdir())
        manifest = json.loads((snapshot / "snapshot.json").read_text())
        for case in ("image", "revision", "timestamp", "symlink"):
            with self.subTest(case=case):
                coverage.write_json(status_file, status)
                coverage.write_json(snapshot / "snapshot.json", manifest)
                if case in ("image", "revision"):
                    coverage.write_json(status_file, dict(status, **{case: "wrong"}))
                elif case == "timestamp":
                    coverage.write_json(snapshot / "snapshot.json", dict(manifest, collected_at="invalid"))
                else:
                    (snapshot / "unexpected").symlink_to(self.source / "revision")
                target = coverage.Collector(self.path / ("combined-" + case))
                self.assertEqual(coverage.aggregate(target, {"suite": str(suite.output)}), 2)
                self.assertFalse((target.output / "coverage.html").exists())

    def test_http_artifact_manifest_uses_shared_log_root(self):
        suite = self.save_suite("suite")
        payload = json.dumps({"deliverable_blob": {"coverage_directory": str(suite.output)}}).encode()
        with mock.patch.dict(os.environ, {"E2E_COVERAGE_LOG_ROOT": str(self.path)}), \
                mock.patch.object(coverage.urllib.request, "urlopen", return_value=io.BytesIO(payload)):
            self.assertEqual(coverage.aggregate(self.collector, {"suite": "https://test.invalid/blob"}), 0)
        other = coverage.Collector(self.path / "outside")
        with mock.patch.dict(os.environ, {"E2E_COVERAGE_LOG_ROOT": str(self.path / "different-root")}), \
                mock.patch.object(coverage.urllib.request, "urlopen", return_value=io.BytesIO(payload)):
            self.assertEqual(coverage.aggregate(other, {"suite": "https://test.invalid/blob"}), 2)

    def test_malformed_records_fail_completeness_without_losing_valid_suites(self):
        valid = self.save_suite("valid")
        for record in ("status.json", "snapshot.json", "pod.json", "http-list", "http-wrapper"):
            with self.subTest(record=record):
                suite = self.save_suite("malformed-" + record)
                location = str(suite.output)
                payload = b"[]" if record == "http-list" else b'{"deliverable_blob": null}'
                if record.startswith("http-"):
                    location = "https://test.invalid/blob"
                else:
                    path = suite.output / record if record == "status.json" else next(suite.raw.iterdir()) / record
                    path.write_text("[]")
                target = coverage.Collector(self.path / ("combined-" + record))
                with mock.patch.object(coverage.urllib.request, "urlopen", return_value=io.BytesIO(payload)):
                    code = coverage.aggregate(target, {"valid": str(valid.output), "malformed": location})
                self.assertEqual(code, 2)
                status = json.loads((target.output / "status.json").read_text())
                self.assertTrue(status["report_generated"])
                self.assertFalse(status["coverage_passed"])
                self.assertEqual(status["expected_suites"], 2)
                self.assertEqual(len(status["errors"]), 1)

    def test_snapshot_order_preserves_nanosecond_ordering(self):
        self.assertLess(coverage.snapshot_order({"collected_at": "2026-10-21T12:00:00.000000001Z"}),
                        coverage.snapshot_order({"collected_at": "2026-10-21T12:00:00.000000002Z"}))


if __name__ == "__main__":
    unittest.main()
