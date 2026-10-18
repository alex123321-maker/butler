#!/usr/bin/env python3
"""Synthetic helper controls: every Docker subprocess is replaced before the
auth helper runs. No real main/private DB, auth, token or network is accessed.
"""
import base64
import contextlib
import io
import json
import os
import pathlib
import runpy
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parent
OLD = base64.b64encode(b"synthetic-opaque-original-00000000000000").decode()
NEW = base64.b64encode(b"synthetic-opaque-rotated-111111111111111").decode()


class Controls(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="issue87-native-helper-control-", dir="/srv/storage/butler/agents")
        self.addCleanup(self.temp.cleanup)
        self.base = pathlib.Path(self.temp.name)
        self.proof = self.base / "reports" / "native-v5"
        self.proof.mkdir(parents=True)
        (self.proof / "prepared").write_text("PASS\n")
        (self.proof / "scheduled-before-auth.json").write_text("{}")
        self.private = self.base / "private" / "native-v5"

    def write_private(self, name, value):
        self.private.mkdir(parents=True, exist_ok=True)
        path = self.private / name
        path.write_text(value)
        path.chmod(0o600)

    def invoke(self, mode, response):
        output = io.StringIO()
        def fake(args, **kwargs):
            self.assertEqual(kwargs["timeout"], 40)
            self.assertTrue(kwargs["capture_output"])
            return subprocess.CompletedProcess(args, 0, response(args, kwargs.get("input", "")), "PRIVATE_UNTRUSTED_STDERR")
        with mock.patch.object(sys, "argv", ["helper", mode, str(self.base), "native-v5"]), mock.patch("subprocess.run", side_effect=fake), contextlib.redirect_stdout(output):
            runpy.run_path(str(ROOT / "memory-adaptive-auth.py"), run_name="__main__")
        self.assertNotIn(OLD, output.getvalue())
        self.assertNotIn(NEW, output.getvalue())
        self.assertNotIn("PRIVATE_UNTRUSTED_STDERR", output.getvalue())
        return json.loads(output.getvalue())

    def test_utc_positive_offset_boundary(self):
        utc_day = runpy.run_path(str(ROOT / "memory-adaptive-history.py"))["utc_day"]
        self.assertEqual(utc_day("2026-10-18T00:30:00+07:00"), "2026-10-17")
        self.assertEqual(utc_day("2026-10-18T07:30:00+07:00"), "2026-10-18")
        self.assertEqual(utc_day("2026-10-17T23:30:00-07:00"), "2026-10-18")

    def test_unused_precollector_clone_requires_zero(self):
        self.write_private("expected-main.value", OLD)
        self.write_private("auth-import.value", OLD)
        result = self.invoke("recover", lambda args, sql: json.dumps({"settings": False, "entries": False, "reservations": False}))
        self.assertTrue(result["proven_unused_clone"])
        self.assertEqual((self.private / "auth-return.value").read_text(), OLD)

    def test_started_without_latest_auth_denies_clone_fallback(self):
        self.write_private("expected-main.value", OLD)
        self.write_private("auth-import.value", OLD)
        def response(args, sql):
            if "to_regclass" in sql:
                return json.dumps({"settings": True, "entries": True, "reservations": True})
            return "1" if "count(*)" in sql else ""
        with self.assertRaisesRegex(SystemExit, "collector may have used auth"):
            self.invoke("recover", response)
        self.assertFalse((self.private / "auth-return.value").exists())

    def test_rotated_isolated_value_wins(self):
        self.write_private("expected-main.value", OLD)
        self.write_private("auth-import.value", OLD)
        result = self.invoke("recover", lambda args, sql: json.dumps({"settings": True, "entries": True, "reservations": True}) if "to_regclass" in sql else NEW)
        self.assertTrue(result["isolated_latest_exported"])
        self.assertEqual((self.private / "auth-return.value").read_text(), NEW)

    def test_bounded_process_timeout_exposes_fixed_diagnostic_only(self):
        def response(args, sql):
            raise subprocess.TimeoutExpired(args, 40, output="PRIVATE_UNTRUSTED_BACKEND")
        with self.assertRaisesRegex(SystemExit, "^bounded opaque handoff process unavailable$"):
            self.invoke("recover", response)
        self.assertFalse((self.private / "auth-return.value").exists())

    def test_return_exact_original_cas_and_no_active_work(self):
        self.write_private("expected-main.value", OLD)
        self.write_private("auth-return.value", NEW)
        def response(args, sql):
            self.assertIn(NEW+"\tnew", sql)
            self.assertIn(OLD+"\texpected", sql)
            self.assertIn("AND value=(SELECT value", sql)
            self.assertIn("session_ownership", sql)
            self.assertIn("accepted_inputs", sql)
            return ""
        self.assertTrue(self.invoke("return", response)["opaque_main_cas"])

    def test_clone_saves_only_opaque_and_matching_private_environment(self):
        def response(args, sql):
            if "inspect" in args:
                return json.dumps([{"Config": {"Env": ["BUTLER_SETTINGS_ENCRYPTION_KEY=synthetic-key", "BUTLER_POSTGRES_URL=synthetic-db", "HTTPS_PROXY=http://127.0.0.1:18080"]}}])
            return OLD if "SELECT value" in sql else "0"
        self.assertTrue(self.invoke("clone", response)["opaque_clone_saved"])
        self.assertEqual((self.private / "expected-main.value").read_text(), OLD)
        self.assertEqual((self.private / "native.env").stat().st_mode & 0o777, 0o600)

    def outcome_fixture(self):
        fixture = pathlib.Path(os.environ["BUTLER_ADAPTIVE_OUTCOME_CONTROL"])
        for name in ("scheduled-before-auth.json", "inputs.json", "measurements.json"):
            shutil.copyfile(fixture / name, self.proof / name)
        initial = (self.proof / "scheduled-before-auth.json").read_bytes()
        (self.proof / "all-original-outcomes.json").write_bytes(initial)
        return runpy.run_path(str(ROOT / "memory-adaptive-outcomes.py"))["export"], initial

    def test_full96_actual_go_digests_and_nullable_unexecuted_export(self):
        export, _ = self.outcome_fixture()
        self.assertEqual(export(self.proof)["full_originals"], 96)
        actual = json.loads((self.proof / "all-original-outcomes.json").read_bytes())
        self.assertEqual(len(actual["observations"]), 96)
        self.assertTrue(all(o["milliseconds"] is None for o in actual["observations"]))

    def test_partial_duplicate_and_forged_capture_never_replace_full96(self):
        export, initial = self.outcome_fixture()
        path = self.proof / "measurements.json"
        original = json.loads(path.read_bytes())
        for kind in ("partial", "duplicate", "forged_hash"):
            changed = json.loads(json.dumps(original))
            observations = changed["observations"]
            if kind == "partial":
                observations.pop()
            elif kind == "duplicate":
                observations[-1] = observations[0]
            else:
                observations[0]["input_sha256"] = "0"*64
            path.write_text(json.dumps(changed))
            with self.assertRaises(SystemExit):
                export(self.proof)
            self.assertEqual((self.proof / "all-original-outcomes.json").read_bytes(), initial)


if __name__ == "__main__":
    unittest.main()
