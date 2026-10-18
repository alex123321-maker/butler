#!/usr/bin/env python3
"""No-auth controls for exited-collector and opaque-file recovery fences."""
import base64
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location('temporal_recovery', pathlib.Path(__file__).with_name('memory-temporal-auth-recovery.py'))
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class RecoveryControls(unittest.TestCase):
    def result(self, code=0, value=''):
        return subprocess.CompletedProcess([], code, value, '')

    def test_running_unknown_or_unavailable_process_state_denies_return(self):
        for results in ([self.result(1)], [self.result(value='collector\n'), self.result(1)], [self.result(value='collector\n'), self.result(value=json.dumps([{'State': {'Running': True, 'Status': 'running'}}]))], [self.result(value='collector\n'), self.result(value='[]')], [self.result(value='collector\n'), self.result(value=json.dumps([{'State': {'Running': False}}]))]):
            with self.subTest(results=results):
                with patch.object(recovery.subprocess, 'run', side_effect=results), self.assertRaises(SystemExit):
                    recovery.collector_exited('collector')

    def test_known_exited_or_absent_collector_allows_further_metadata_checks(self):
        for results in ([self.result()], [self.result(value='collector\n'), self.result(value=json.dumps([{'State': {'Running': False, 'Status': 'exited'}}]))]):
            with self.subTest(results=results):
                with patch.object(recovery.subprocess, 'run', side_effect=results):
                    recovery.collector_exited('collector')

    def test_opaque_file_requires_private_regular_bounded_ciphertext(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory)/'opaque.value'
            value = base64.b64encode(b'synthetic-noncredential-encrypted-row'*2)
            path.write_bytes(value)
            path.chmod(0o600)
            self.assertEqual(recovery.opaque(path), value)
            path.chmod(0o644)
            with self.assertRaises(SystemExit):
                recovery.opaque(path)
            path.chmod(0o600)
            path.write_bytes(b'not encrypted base64')
            with self.assertRaises(SystemExit):
                recovery.opaque(path)
            path.write_bytes(value)
            link = pathlib.Path(directory)/'link.value'
            os.symlink(path, link)
            with self.assertRaises(OSError):
                recovery.opaque(link)
            path.write_bytes(b'x'*(1024*1024+1))
            with self.assertRaises(SystemExit):
                recovery.opaque(path)


if __name__ == '__main__':
    unittest.main()
