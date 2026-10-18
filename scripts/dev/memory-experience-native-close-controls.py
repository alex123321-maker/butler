#!/usr/bin/env python3
"""Offline controls for companion receipt population/NULL boundaries."""
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('experience_close', pathlib.Path(__file__).with_name('memory-experience-native-close.py'))
close = importlib.util.module_from_spec(spec)
spec.loader.exec_module(close)


class ClosureControls(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.proof = pathlib.Path(self.tmp.name)
        (self.proof/'plan').mkdir()
        (self.proof/'history').mkdir()
        self.actual = self.proof/'actual'
        self.actual.mkdir()
        self.slots = [{'sequence': i, 'id': 'slot-'+str(i), 'kind': 'original' if i < 5 else 'control', 'case_id': str(i), 'arm': 'shared', 'repetition': 0, 'maximum_provider_entries': 12} for i in range(1, 58)]
        # The actual corpus uses four one-entry notes; the denominator stays57.
        for slot in self.slots[4:8]:
            slot['maximum_provider_entries'] = 1
        self.old = {'native_entries': 0, 'native_known_usage': 0, 'native_unknown_usage': 0, 'native_missing_total': 0, 'known_full_token_lower_bound': 0, 'product_attempts': 3, 'product_observed': 0, 'product_null': 3, 'product_without_native_entry': 3}
        self.put('plan/preseal.json', {'owner': 'owner', 'deadline': '2026-10-18T23:59:59Z', 'schedule': self.slots, 'history': {'day': '2026-10-18', 'sha256': 'a'*64, 'inclusive_usage': self.old}})
        self.expected = hashlib.sha256((self.proof/'plan/preseal.json').read_bytes()).hexdigest()
        self.put('plan/schedule.json', {'slots': self.slots})
        self.put('history/history.json', {'original': 'independently-validated-by-entry'})
        self.put('runner.jsonl', {})
        (self.proof/'native-status').write_text('1')
        (self.proof/'handback-status').write_text('0')
        self.row = {'operation_id': 'op-1', 'reservation_id': 'res-1', 'run_id': 'run-1', 'observed_at': None, 'actual_input_tokens': None, 'actual_output_tokens': None}
        self.db = {'execution_model_attempts': [self.row], 'memory_model_attempts': []}
        self.put('actual/source-database.json', self.db)
        self.costs = {'allocated_database_exports': ['source-database.json'], 'quotes': {'new_model_reservation_rows': 1, 'inclusive_quote': {'model_turns': 4}}}
        self.put('actual/unique-quoted-costs.json', self.costs)

    def put(self, path, value):
        (self.proof/path).write_text(json.dumps(value))

    def test_zero_native_charged_product_and_old_nulls_remain_independent(self):
        self.put('actual/window-final.json', {'slots': [{**x, 'status': 'not_executed'} for x in self.slots], 'entries': []})
        result = close.summarize(self.proof, self.expected)
        self.assertTrue(result['cost_population_complete'])
        self.assertEqual(result['usage']['inclusive']['product_attempts'], 4)
        self.assertEqual(result['usage']['inclusive']['product_null'], 4)
        self.assertEqual(result['usage']['inclusive']['native_entries'], 0)
        self.assertEqual(result['usage']['new']['product_without_native_entry'], 1)

    def test_native_unknown_does_not_become_zero_usage(self):
        entry = {'operation_id': 'op-1', 'reservation_id': 'res-1', 'run_id': 'run-1', 'usage': None}
        self.put('actual/window-final.json', {'slots': [{**x, 'status': 'failed'} for x in self.slots], 'entries': [entry]})
        result = close.summarize(self.proof, self.expected)
        self.assertEqual(result['usage']['new']['native_unknown_usage'], 1)
        self.assertEqual(result['usage']['new']['known_full_token_lower_bound'], 0)
        self.assertEqual(result['usage']['new']['product_null'], 1)

    def test_known_total_uses_reported_full_total_not_input_output_sum(self):
        self.row.update({'observed_at': 'observed', 'actual_input_tokens': 3, 'actual_output_tokens': 2})
        self.put('actual/source-database.json', self.db)
        entry = {'operation_id': 'op-1', 'reservation_id': 'res-1', 'run_id': 'run-1', 'usage': {'InputTokens': 3, 'OutputTokens': 2, 'TotalTokens': 11}}
        self.put('actual/window-final.json', {'slots': [{**x, 'status': 'completed'} for x in self.slots], 'entries': [entry]})
        result = close.summarize(self.proof, self.expected)
        self.assertEqual(result['usage']['new']['known_full_token_lower_bound'], 11)
        self.assertEqual(result['usage']['new']['product_observed'], 1)

    def test_background_product_run_is_operation_and_remains_separate(self):
        self.db = {'execution_model_attempts': [], 'memory_model_attempts': [{'operation_id': 'op-1', 'reservation_id': 'res-1', 'observed_at': None, 'usage': None}]}
        self.put('actual/source-database.json', self.db)
        self.assertEqual(close.product_population(self.actual, self.costs, [{'operation_id': 'op-1', 'reservation_id': 'res-1', 'run_id': 'op-1', 'usage': None}]), (1, 0))
        with self.assertRaises(ValueError):
            close.product_population(self.actual, self.costs, [{'operation_id': 'op-1', 'reservation_id': 'res-1', 'run_id': 'run-1', 'usage': None}])

    def test_abrupt_started_slot_stays_unknown_and_cost_incomplete(self):
        self.put('actual/slot-001-start.json', self.slots[0])
        result = close.summarize(self.proof, self.expected)
        self.assertFalse(result['cost_population_complete'])
        outcomes = close.read(self.proof/'all-original-outcomes.json')['slots']
        self.assertEqual(len(outcomes), 57)
        self.assertEqual(outcomes[0]['status'], 'unknown_collector_terminated')
        self.assertEqual(outcomes[1]['status'], 'not_executed')

    def test_missing_allocated_export_and_changed_copied_product_fail_closed(self):
        self.costs['allocated_database_exports'].append('case-010-database.json')
        with self.assertRaises(OSError):
            close.product_population(self.actual, self.costs, [])
        self.put('actual/case-010-database.json', self.db)
        self.assertEqual(close.product_population(self.actual, self.costs, []), (1, 0))
        self.put('actual/case-010-database.json', {'execution_model_attempts': [{**self.row, 'observed_at': 'changed'}], 'memory_model_attempts': []})
        with self.assertRaises(ValueError):
            close.product_population(self.actual, self.costs, [])

    def test_forged_native_reservation_and_unjoined_quote_are_rejected(self):
        with self.assertRaises(ValueError):
            close.product_population(self.actual, self.costs, [{'operation_id': 'op-1', 'reservation_id': 'other', 'run_id': 'run-1'}])
        self.costs['quotes']['new_model_reservation_rows'] = 2
        with self.assertRaises(ValueError):
            close.product_population(self.actual, self.costs, [])


if __name__ == '__main__':
    unittest.main()
