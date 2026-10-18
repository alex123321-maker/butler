#!/usr/bin/env python3
"""Narrow stdlib no-auth controls for whole92 recovery/history/closure fences."""
import base64
import hashlib
import importlib.util
import json
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

os.environ['PYTHONDONTWRITEBYTECODE'] = '1'
ROOT = pathlib.Path(__file__).parent


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT/filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


recovery = module('coherent_recovery', 'memory-coherent-auth-recovery.py')
closure = module('coherent_closure', 'memory-coherent-native-close.py')
history = module('coherent_history', 'memory-coherent-native-history.py')


class RecoveryControls(unittest.TestCase):
    def setUp(self):
        # TemporaryDirectory owns only a fresh clearly named test directory.
        self.directory = tempfile.TemporaryDirectory(prefix='issue92-native-recovery-control-', dir='/srv/storage/butler/agents')
        self.addCleanup(self.directory.cleanup)
        self.base = pathlib.Path(self.directory.name)
        self.label = 'synthetic'
        self.proof, self.private = self.base/'reports'/self.label, self.base/'private'/self.label
        self.proof.mkdir(parents=True, mode=0o700)
        self.private.mkdir(parents=True, mode=0o700)
        self.plan = b'{"synthetic_contract":true}\n'
        (self.proof/'preseal.json').write_bytes(self.plan)
        self.sha = hashlib.sha256(self.plan).hexdigest()
        self.old = base64.b64encode(b'synthetic-old-encrypted-fixture'*2)
        self.latest = base64.b64encode(b'synthetic-new-encrypted-fixture'*2)
        for name in ('auth-import.value', 'expected-main.value'):
            (self.private/name).write_bytes(self.old)
            (self.private/name).chmod(0o600)
        self.value, self.count = self.latest.decode(), 0
        self.queries = []

    def sql(self, label, query):
        self.assertEqual(label, self.label)
        self.assertNotIn('restore', query)
        self.queries.append(query)
        if 'coalesce(jsonb_agg' in query:
            return [{'preseal_sha256': self.sha, 'schema_name': 'coherent_native_v1', 'owner_id': 'owner'}]
        if "jsonb_build_object('value'" in query:
            return {'value': self.value}
        if 'SELECT count(*)' in query:
            return self.count
        if 'to_reg' in query:
            return True
        raise AssertionError('unexpected metadata query')

    def run_recovery(self):
        with patch.object(recovery.fences, 'collector_exited') as stopped, patch.object(recovery, 'sql', side_effect=self.sql):
            result = recovery.recover(self.base, self.label, self.sha)
            stopped.assert_called_once_with('butler-issue92-synthetic-native')
            return result

    def test_latest_original_after_process_exit_wins_over_stale_import(self):
        self.assertTrue(self.run_recovery()['opaque_return_ready'])
        self.assertEqual((self.private/'auth-return.value').read_bytes(), self.latest)
        self.assertTrue(json.loads((self.proof/'auth-recovery-safe.json').read_text())['restored_auth_ignored'])
        with self.assertRaises(SystemExit):
            self.run_recovery()

    def test_unproven_exit_denies_before_any_sql_or_return(self):
        with patch.object(recovery.fences, 'collector_exited', side_effect=SystemExit('unknown process')), patch.object(recovery, 'sql') as query, self.assertRaises(SystemExit):
            recovery.recover(self.base, self.label, self.sha)
        query.assert_not_called()
        self.assertFalse((self.private/'auth-return.value').exists())

    def test_missing_auth_with_admission_or_import_marker_denies_fallback(self):
        self.value, self.count = None, 1
        with self.assertRaises(SystemExit):
            self.run_recovery()
        self.count = 0
        (self.proof/'provider-auth-imported.json').write_text('{}')
        with self.assertRaises(SystemExit):
            self.run_recovery()

    def test_unused_pre_import_original_can_be_returned_only_when_equal(self):
        self.value = None
        self.assertEqual(self.run_recovery()['source'], 'unused_original_before_provider_import')
        self.assertEqual((self.private/'auth-return.value').read_bytes(), self.old)

    def test_malformed_latest_or_changed_preseal_never_uses_old_value(self):
        self.value = 'malformed'
        with self.assertRaises(SystemExit):
            self.run_recovery()
        self.value = self.latest.decode()
        (self.proof/'preseal.json').write_bytes(self.plan+b'changed')
        with self.assertRaises(SystemExit):
            self.run_recovery()


class HistoryControls(unittest.TestCase):
    def test_prior92_window_denies_even_a_new_label_before_any_inventory_query(self):
        with patch.object(history.glob, 'glob', return_value=['prior92/auth-stage-begun.json']), patch.object(history.util, 'current') as current, self.assertRaises(SystemExit):
            history.main()
        current.assert_not_called()

    def test_nested_action_order_is_preserved_but_relation_order_is_not(self):
        self.assertEqual(history.relation([{'id': 1}, {'id': 2}]), history.relation([{'id': 2}, {'id': 1}]))
        self.assertNotEqual(history.relation([{'actions': [1, 2]}]), history.relation([{'actions': [2, 1]}]))

    def test_path_escape_symlink_and_changed_hash_deny(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            path = root/'body.json'
            path.write_bytes(b'[]')
            sha = hashlib.sha256(b'[]').hexdigest()
            self.assertEqual(history.reference(root, {'path': 'body.json', 'sha256': sha})[1], b'[]')
            for reference in ({'path': '../body.json', 'sha256': sha}, {'path': 'body.json', 'sha256': '0'*64}):
                with self.assertRaises(SystemExit):
                    history.reference(root, reference)
            (root/'linked.json').symlink_to(path)
            with self.assertRaises(SystemExit):
                history.reference(root, {'path': 'linked.json', 'sha256': sha})


class ClosureControls(unittest.TestCase):
    def test_projection_excludes_session_transport_and_arbitrary_metadata(self):
        request={'Context':{'RunID':'run','SessionKey':'private-session','ProviderSessionRef':{'SessionRef':'private-provider-session'},'ModelName':'model'},'InputItems':[{'Role':'user','Content':'actual DATA','Metadata':{'arbitrary':'private','butler_provenance':{'kind':'document','source_refs':['original'],'sensitivity':'private'}}}], 'TransportOptions':{'unknown':'private'}}
        safe=closure.safe_request(request,'start')
        self.assertEqual(safe['Context'],{'RunID':'run','ModelName':'model'})
        self.assertEqual(safe['InputItems'][0]['Metadata'],{'butler_provenance':request['InputItems'][0]['Metadata']['butler_provenance']})
        self.assertNotIn('TransportOptions',safe)
        self.assertIn('ProviderSessionRef',request['Context'])

    def test_handback_requires_exited_original_recovery_and_actual_discovery200(self):
        source='latest_original_owning_durable_encrypted_setting_after_process_exit'
        full={'native_status':0,'handback_status':0,'collector_exit':{'Running':False,'Status':'exited'},'recovery':{'opaque_return_ready':True,'source':source,'raw_material_printed':False},'recovery_fence':{'collector_running':False,'original_database':'coherent_native_test','restored_auth_ignored':True,'source':source},'return':{'opaque_main_cas':True,'active_work_checked':True},'main_control_plane':{'resolved':True,'unsafe_refresh':False,'expired':False,'trace':[{'stage':'paid_discovery','outcome':'completed','http_status':200}]},'main_snapshot':{'fresh_main_snapshot_saved':True}}
        self.assertTrue(closure.complete_handback(full))
        for key in ('collector_exit','recovery','recovery_fence','return','main_control_plane','main_snapshot'):
            forged=dict(full);forged[key]={}
            self.assertFalse(closure.complete_handback(forged))
        forged=json.loads(json.dumps(full));forged['main_control_plane']['trace'][0]['http_status']=503
        self.assertFalse(closure.complete_handback(forged))

    def test_full_native_usage_is_measured_not_visible_token_sum_and_sequence_is_exact(self):
        entries=[{'sequence':1,'slot':'slot','operation_id':'one','reservation_id':'r1','usage':{'InputTokens':3,'OutputTokens':2,'TotalTokens':8}},{'sequence':2,'slot':'slot','operation_id':'two','reservation_id':'r2','usage':None},{'sequence':3,'slot':'slot','operation_id':'three','reservation_id':'r3','usage':{'InputTokens':1,'OutputTokens':0,'TotalTokens':0}}]
        usage,counts=closure.native_usage(entries,[{'name':'slot','maximum_entries':3}])
        self.assertEqual(usage['known_full_token_lower_bound'],8)
        self.assertEqual(usage['native_unknown_usage'],1)
        self.assertEqual(usage['native_missing_total'],1)
        self.assertEqual(counts['slot'],3)
        entries[1]['sequence']=1
        with self.assertRaises(ValueError):
            closure.native_usage(entries,[{'name':'slot','maximum_entries':3}])

    def test_complete_synthetic_closure_rejects_changed_usage_or_checkpoint_order(self):
        # Pure closure-unit fixture, not an actual provider or acceptance receipt.
        source='latest_original_owning_durable_encrypted_setting_after_process_exit'
        for mutation in ('none','full_total','checkpoint_order'):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                proof=pathlib.Path(directory)
                (proof/'history').mkdir();(proof/'timeline').mkdir()
                schedule=[{'name':str(index),'maximum_entries':5,'state':'completed'} for index in range(15)]
                entries=[{'sequence':index+1,'slot':str(index),'operation_id':'op'+str(index),'reservation_id':'r'+str(index),'kind':'start','request':{'Context':{'RunID':'run'+str(index)}},'usage':{'InputTokens':3,'OutputTokens':2,'TotalTokens':8},'events':[]} for index in range(15)]
                measured,_=closure.native_usage(entries,schedule)
                new={**measured,'product_attempts':20,'product_without_native_entry':5,'product_observed':15,'product_null':5}
                costs={'new_usage':new,'inclusive_usage':dict(new),'inclusive_quote':{'model_turns':20}}
                if mutation=='full_total':costs['new_usage']['known_full_token_lower_bound']-=1
                checkpoints=['checkpoint'+str(index) for index in range(25)]
                actual=list(checkpoints)
                if mutation=='checkpoint_order':actual[0],actual[1]=actual[1],actual[0]
                seal={'format':'butler.coherent.native-preseal.v1','owner':'owner','deadline':'synthetic','history':{'day':'synthetic','sha256':'b'*64},'schedule':schedule,'ordered_checkpoints':checkpoints,'controlled_product_admissions_maximum':5}
                files={'preseal.json':seal,'scheduled-before-auth.json':schedule,'history/history.json':{},'native-window-final.json':{'entries':entries,'slots':schedule,'capture_error':False,'whole_failed':False,'mechanics_tail_reached':True},'native-outcome.json':{'cost_population_complete':True,'mechanical_pass':True},'timeline/timeline-lifecycle.json':{'pass':True,'native_calls':15,'steps':[{'Name':name} for name in actual]},'full-quoted-costs.json':costs,'collector-exit-safe.json':{'Running':False,'Status':'exited'},'recovery-safe.json':{'opaque_return_ready':True,'source':source,'raw_material_printed':False},'auth-recovery-safe.json':{'collector_running':False,'original_database':'coherent_native_test','restored_auth_ignored':True,'source':source},'return-safe.json':{'opaque_main_cas':True,'active_work_checked':True},'main-auth-safe.json':{'resolved':True,'unsafe_refresh':False,'expired':False,'trace':[{'stage':'paid_discovery','outcome':'completed','http_status':200}]},'main-snapshot-safe.json':{'fresh_main_snapshot_saved':True}}
                for name,value in files.items():(proof/name).write_text(json.dumps(value))
                (proof/'native-status').write_text('0');(proof/'handback-status').write_text('0')
                sha=hashlib.sha256((proof/'preseal.json').read_bytes()).hexdigest()
                if mutation=='full_total':
                    with self.assertRaises(ValueError):closure.close(proof,sha)
                    self.assertFalse((proof/'closed-whole-metadata.json').exists())
                else:
                    result=closure.close(proof,sha)
                    self.assertEqual(result['mechanical_pass'],mutation=='none')
                    self.assertFalse(result['release_qualified'])

    def test_aborted_zero_entry_window_keeps_all15_unknown_cost_and_no_go(self):
        with tempfile.TemporaryDirectory() as directory:
            proof = pathlib.Path(directory)
            (proof/'history').mkdir()
            seal = {'format': 'butler.coherent.native-preseal.v1', 'owner': 'owner', 'deadline': 'synthetic', 'history': {'day': 'synthetic', 'sha256': 'b'*64}, 'schedule': [{'name': str(index), 'maximum_entries': 5} for index in range(15)], 'ordered_checkpoints': list(range(25)), 'controlled_product_admissions_maximum': 5}
            for path, value in [(proof/'preseal.json', seal), (proof/'scheduled-before-auth.json', seal['schedule']), (proof/'history/history.json', {})]:
                path.write_text(json.dumps(value))
            (proof/'native-status').write_text('1')
            (proof/'handback-status').write_text('0')
            sha = hashlib.sha256((proof/'preseal.json').read_bytes()).hexdigest()
            result = closure.close(proof, sha)
            self.assertFalse(result['mechanical_pass'])
            self.assertFalse(result['cost_population_complete'])
            self.assertIsNone(result['usage'])
            self.assertEqual(len(json.loads((proof/'all-original-outcomes.json').read_text())['slots']), 15)
            self.assertFalse(result['release_qualified'])
            with self.assertRaises(FileExistsError):
                closure.close(proof, sha)


if __name__ == '__main__':
    unittest.main()
