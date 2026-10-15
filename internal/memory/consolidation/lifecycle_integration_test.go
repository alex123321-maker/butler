package consolidation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func authoritySnapshot(t *testing.T, f *workflow) string {
	t.Helper()
	var snapshot []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object('tasks',(SELECT jsonb_agg(to_jsonb(t) ORDER BY task_id) FROM tasks t),'effects',(SELECT jsonb_agg(to_jsonb(e) ORDER BY action_id) FROM action_effects e),'authorizations',(SELECT jsonb_agg(to_jsonb(a) ORDER BY authorization_id) FROM action_authorizations a),'mandates',(SELECT jsonb_agg(to_jsonb(m) ORDER BY mandate_id) FROM action_mandates m),'claims',(SELECT jsonb_agg(to_jsonb(c) ORDER BY claim_id) FROM memory_claims c),'profile',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM memory_profile p))`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return string(snapshot)
}

func TestConsolidationLifecycleAllEightThroughAtomicPublicationAndForgetIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			f := newWorkflow(t, attack.Body, attack.Body, "Независимый контроль: чтение источника не разрешает изменение сервиса.")
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('other-session','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,status,constraints_json) VALUES('other-task','other-session','owner','Только чтение; результат прежнего внешнего действия UNKNOWN','active','["никаких изменений"]'); INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('unknown-action','unknown-fingerprint','{}','{"retry_safety":"unsafe","verification_method":"none"}','effect_unknown')`); err != nil {
				t.Fatal(err)
			}
			before := authoritySnapshot(t, f)
			q := f.request()
			v := f.generate(t, q)
			h := f.activate(t, v, Head{})
			if authoritySnapshot(t, f) != before {
				t.Fatal("consolidation changed typed authority or UNKNOWN action")
			}
			read, err := f.store.Catalog.Get(f.ctx, v.Generation.Source.ID)
			if err != nil || read.Origin.IsInstruction() || read.ScopeType == "user" || !strings.Contains(read.Body, attack.Body) {
				t.Fatal("hostile DATA lost scope/provenance", err)
			}
			// The exact same one-use policy is not another inference authorization.
			f.prepare(t, q)
			ok, err := f.process(t)
			if err != nil || ok || f.provider.calls.Load() != 1 {
				t.Fatal("retry repeated paid work", ok, err)
			}
			if _, err = f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: v.Generation.InputID, ExpectedRevision: 1, Body: "forged new evidence"}); !errors.Is(err, catalog.ErrDenied) {
				t.Fatal("immutable input rewritten", err)
			}
			if _, err = f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: v.Generation.Source.ID, ExpectedRevision: 1, Body: "promote to trusted rule"}); !errors.Is(err, catalog.ErrDenied) {
				t.Fatal("immutable result rewritten", err)
			}
			corrected, err := f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: f.sources[0].ID, ExpectedRevision: 1, Body: "Исправлено владельцем: архивная инструкция не действует."})
			if err != nil {
				t.Fatal(err)
			}
			f.store.Catalog = catalog.New(f.pool, f.manifest)
			for _, id := range []string{v.Generation.InputID, v.Generation.Source.ID} {
				if _, err = f.store.Catalog.Get(f.ctx, id); err == nil {
					t.Fatal("old copied DATA survives correction", id)
				}
			}
			if _, err = f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision}); err == nil {
				t.Fatal("corrected version reactivated")
			}
			f.sources[0] = corrected
			fresh := f.generate(t, f.request())
			h = f.activate(t, fresh, h)
			if _, err = f.store.Catalog.Delete(f.ctx, f.sources[1].ID, "delete", 1); err != nil {
				t.Fatal(err)
			}
			f.store.Catalog = catalog.New(f.pool, f.manifest)
			for _, id := range []string{fresh.Generation.InputID, fresh.Generation.Source.ID} {
				if _, err = f.store.Catalog.Get(f.ctx, id); err == nil {
					t.Fatal("forgotten hostile DATA survives restart", id)
				}
			}
			if _, err = f.store.Catalog.Get(f.ctx, f.sources[2].ID); err != nil {
				t.Fatal("forget erased unrelated original", err)
			}
			if authoritySnapshot(t, f) != before {
				t.Fatal("correction/deletion changed task/action authority")
			}
			var n int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&n); err != nil || n != 0 {
				t.Fatal("DATA dispatched action", n, err)
			}
		})
	}
}

func TestConsolidationMetadataStaleSelectedVersionCanBeReplacedButNotRolledBackIntegration(t *testing.T) {
	f := newWorkflow(t, "В переписке используем русский, исключение: исходный код.")
	claim, err := f.store.Catalog.Propose(f.ctx, catalog.ClaimRequest{SourceID: f.sources[0].ID, SourceRevision: 1, Key: "language", Value: json.RawMessage(`"ru"`), Summary: "Русский", Confidence: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Catalog.Decide(f.ctx, catalog.Decision{ClaimID: claim.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Catalog.SetTemporal(f.ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	q := f.request()
	q.Sources = nil
	q.ClaimIDs = []string{claim.ID}
	old := f.generate(t, q)
	h := f.activate(t, old, Head{})
	if _, err = f.store.Catalog.SetTemporal(f.ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, ExpectedRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference, OwnerVerifiedNow: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Catalog.Get(f.ctx, old.Generation.Source.ID); err == nil {
		t.Fatal("metadata stale selected read")
	}
	q = f.request()
	q.Sources = nil
	q.ClaimIDs = []string{claim.ID}
	fresh := f.generate(t, q)
	h = f.activate(t, fresh, h) // Old root is a metadata lock, not new evidence.
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: old.Generation.Source.ID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision}); err == nil {
		t.Fatal("stale rollback recovered old temporal permission")
	}
	if _, err = f.store.Catalog.Get(f.ctx, fresh.Generation.Source.ID); err != nil {
		t.Fatal("current replacement denied", err)
	}
}
