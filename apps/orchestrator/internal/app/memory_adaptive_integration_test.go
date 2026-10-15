package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
)

type capturedAdaptiveRead struct {
	readcontrol.Service
	errors chan error
}

type adaptiveSemanticFixture struct {
	search func(context.Context, []memoryservice.Scope, string, int, memoryservice.QueryPolicy) (memoryservice.SearchResult, error)
}

func (s adaptiveSemanticFixture) SearchWithPolicy(ctx context.Context, scopes []memoryservice.Scope, q string, n int, p memoryservice.QueryPolicy) (memoryservice.SearchResult, error) {
	return s.search(ctx, scopes, q, n, p)
}

func TestAdaptiveSemanticIPCOutsideReadTransactionAndLateGuardIntegration(t *testing.T) {
	for _, mutation := range []string{"unchanged", "correction_during_ipc", "stop_during_ipc"} {
		t.Run(mutation, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Search)
			source := f.args.Sources[0]
			if _, err := f.f.sources.PublishFragments(f.f.owner, catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}, []catalog.Fragment{{Key: "semantic", Text: "проверить actual evidence", Locator: json.RawMessage(`{}`)}}); err != nil {
				t.Fatal(err)
			}
			var chunk int64
			var revision string
			if err := f.f.pool.QueryRow(f.f.ctx, `SELECT c.id,to_char(c.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM memory_chunks c JOIN memory_object_sources o ON o.object_id=c.id AND o.memory_type='chunk' WHERE o.source_id=$1`, source.ID).Scan(&chunk, &revision); err != nil {
				t.Fatal(err)
			}
			if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
				t.Fatal(err)
			}
			f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
			f.call.ToolCallId = "adaptive-semantic-call"
			f.call.ArgsJson = `{"query":"проверить","strategy":"semantic_hybrid","limit":4}`
			var err error
			f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Search))
			if err != nil || f.prepared.RequiresApproval {
				t.Fatal(err)
			}
			f.grant, err = f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Search), f.prepared.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			store, err := adaptive.NewStore(f.f.pool, f.f.sources)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Init(f.owned, adaptive.Classify("Вспомни наши записи")); err != nil {
				t.Fatal(err)
			}
			prepareMemoryReadCheckpoint(t, f)
			called := make(chan struct{}, 1)
			backend := adaptiveSemanticFixture{search: func(ctx context.Context, scopes []memoryservice.Scope, q string, n int, p memoryservice.QueryPolicy) (memoryservice.SearchResult, error) {
				defer func() { called <- struct{}{} }()
				if f.f.pool.Stat().AcquiredConns() != 0 {
					t.Error("owning SQL transaction spans semantic IPC")
				}
				if !p.Local || p.External || len(scopes) > 12 || q != "проверить" || n != 4 {
					t.Error("query/authority composition changed")
				}
				var mutationErr error
				switch mutation {
				case "correction_during_ipc":
					_, mutationErr = f.f.sources.Correct(f.f.owner, catalog.Correction{SourceID: source.ID, ExpectedRevision: source.Revision, Body: "Corrected source"})
				case "stop_during_ipc":
					_, mutationErr = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET status='cancelled' WHERE task_id=$1`, f.f.parent.TaskID)
				}
				if mutationErr != nil {
					return memoryservice.SearchResult{}, mutationErr
				}
				return memoryservice.SearchResult{Hits: []memoryservice.SemanticHit{{SourceRef: fmt.Sprintf("memory_chunk:%d", chunk), Revision: revision}}, Diagnostics: []string{"hybrid_base"}}, nil
			}}
			reads, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Store: store, Semantic: backend})
			if err != nil {
				t.Fatal(err)
			}
			h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", reads)
			if err != nil {
				t.Fatal(err)
			}
			process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Search)
			result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
			select {
			case <-called:
			default:
				t.Fatal("semantic path not exercised", err, result)
			}
			if mutation == "stop_during_ipc" {
				if err == nil && result.GetStatus() == "completed" {
					t.Fatal("stopped authority returned search evidence")
				}
				return
			}
			if err != nil || result.GetStatus() != "completed" {
				t.Fatal("search failed", err, result)
			}
			out, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "unchanged" {
				if out.Status != "ok" || len(out.Items) != 1 || out.Items[0].ReadRef != "" || len(out.Diagnostics) == 0 {
					t.Fatal(out)
				}
			} else if len(out.Items) != 0 || out.Status != "empty" || !out.Truncated {
				t.Fatal("late stale semantic candidate escaped", out)
			}
		})
	}
}

func (c capturedAdaptiveRead) ReadDispatched(ctx context.Context, e readcontrol.Envelope) (readcontrol.Result, error) {
	out, err := c.Service.ReadDispatched(ctx, e)
	c.errors <- err
	return out, err
}

func TestAdaptiveActualBrokerOpenedEvidenceIntegration(t *testing.T) {
	f, store, answer := actualAdaptiveOpenedEvidence(t)
	validate := func(a adaptive.Answer) error {
		raw, _ := json.Marshal(a)
		_, err := store.Validate(f.owned, f.call.RunId, string(raw))
		return err
	}
	if err := validate(answer); err != nil {
		t.Fatal("confirmed actual current opened evidence rejected", err)
	}
	for _, mutation := range []func(*adaptive.Answer){
		func(a *adaptive.Answer) { a.Citations[0].ReadRef = "read_" + strings.Repeat("0", 64) },
		func(a *adaptive.Answer) { a.Citations[0].Quote = "invented" },
		func(a *adaptive.Answer) { a.Citations[0].QuoteOffset = 1 },
		func(a *adaptive.Answer) { a.Answer = "See source_unopened" },
		func(a *adaptive.Answer) { a.Citations[0].Claim = "See message:unopened" },
	} {
		copyAnswer := answer
		copyAnswer.Citations = append([]adaptive.Citation(nil), answer.Citations...)
		mutation(&copyAnswer)
		if validate(copyAnswer) == nil {
			t.Fatal("forged evidence accepted")
		}
	}
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE action_effect_evidence SET evidence=jsonb_set(evidence,'{result_digest}',to_jsonb(repeat('0',64))) WHERE action_id=$1`, f.prepared.EffectID); err == nil {
		t.Fatal("permanent Broker evidence became mutable")
	}
	// Corrupting a mutable consumer projection cannot counterfeit an immutable
	// Broker proof. Preserve the actual confirmed journal row as the authority.
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE memory_retrieval_operations SET result_hash=repeat('0',64) WHERE effect_id=$1`, f.prepared.EffectID); err != nil {
		t.Fatal(err)
	}
	if validate(answer) == nil {
		t.Fatal("unconfirmed result digest accepted")
	}
}

func actualAdaptiveOpenedEvidence(t *testing.T) (*dispatchAdmissionFixture, *adaptive.Store, adaptive.Answer) {
	t.Helper()
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	return runAdaptiveOpenedEvidence(t, f)
}

func runAdaptiveOpenedEvidence(t *testing.T, f *dispatchAdmissionFixture) (*dispatchAdmissionFixture, *adaptive.Store, adaptive.Answer) {
	t.Helper()
	store, err := adaptive.NewStore(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Init(f.owned, adaptive.Classify("Вспомни наши записи")); err != nil {
		t.Fatal(err)
	}
	prepareMemoryReadCheckpoint(t, f)
	service, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	observed := capturedAdaptiveRead{Service: service, errors: make(chan error, 1)}
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", observed)
	if err != nil {
		t.Fatal(err)
	}
	process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Open)
	result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || result.GetStatus() != "completed" {
		select {
		case boundaryErr := <-observed.errors:
			t.Log("owning boundary", boundaryErr)
		default:
		}
		t.Fatal("actual Broker read", err, result)
	}
	read, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
	if err != nil || len(read.Items) != 1 || len(read.Items[0].ReadRef) != 69 {
		t.Fatal(read, err)
	}
	item := read.Items[0]
	answer := adaptive.Answer{Answer: "Проверить запись.", Citations: []adaptive.Citation{{Claim: "В записи есть указание", ReadRef: item.ReadRef, Quote: item.Text, QuoteOffset: 0}}, Unknowns: []string{}}
	return f, store, answer
}

func TestAdaptiveLifecycleAttackCorpusDoesNotCreateAuthorityIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
			unrelated := f.args.Sources[0]
			src, err := f.f.sources.Put(f.f.owner, catalog.PutSource{ExternalKey: attack.ID, Kind: "document", ScopeType: "user", ScopeID: f.f.parent.OwnerID, Body: attack.Body, Origin: content.Origin{Kind: content.Kind(attack.OriginKind), Sensitivity: content.Private, SourceRefs: []string{"attack:" + attack.ID}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
				t.Fatal(err)
			}
			f.call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
			f.call.ToolCallId = "attack-open"
			raw, _ := json.Marshal(readcontrol.Args{SourceID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash, MaxChars: 4096})
			f.call.ArgsJson = string(raw)
			f.prepared, err = f.auth.Prepare(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Open))
			if err != nil || f.prepared.RequiresApproval {
				t.Fatal(err)
			}
			f.grant, err = f.auth.Authorize(f.f.ctx, f.call, readcontrol.Contract(readcontrol.Open), f.prepared.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			authority := func() string {
				var value string
				err := f.f.pool.QueryRow(f.f.ctx, `SELECT jsonb_build_object('claims',(SELECT count(*) FROM memory_claims),'grants',(SELECT count(*) FROM action_authorizations),'mandates',(SELECT count(*) FROM action_mandates),'goals',(SELECT count(*) FROM owner_goals),'task',(SELECT to_jsonb(t) FROM tasks t WHERE task_id=$1))::text`, f.f.parent.TaskID).Scan(&value)
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			before := authority()
			_, store, answer := runAdaptiveOpenedEvidence(t, f)
			if answer.Citations[0].Quote != attack.Body {
				t.Fatal("attack not actually opened intact as DATA")
			}
			raw, _ = json.Marshal(answer)
			if _, err = store.Validate(f.owned, f.call.RunId, string(raw)); err != nil {
				t.Fatal(err)
			}
			if authority() != before {
				t.Fatal("retrieval text changed current authority or accepted facts")
			}
			if _, err = f.f.sources.Delete(f.f.owner, src.ID, "delete", src.Revision); err != nil {
				t.Fatal(err)
			}
			restarted, err := adaptive.NewStore(f.f.pool, f.f.sources)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = restarted.Validate(f.owned, f.call.RunId, string(raw)); err == nil {
				t.Fatal("deleted attack receipt replayed after reconstruction")
			}
			if _, err = f.f.sources.Get(f.f.owner, unrelated.ID); err != nil {
				t.Fatal("unrelated source removed", err)
			}
			var body, hash string
			var receipts int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT r.body,r.content_hash,(SELECT count(*) FROM memory_retrieval_opened WHERE source_id=r.source_id) FROM memory_source_revisions r WHERE r.source_id=$1 AND r.revision=$2`, src.ID, src.Revision).Scan(&body, &hash, &receipts); err != nil || body != "" || hash != src.Hash || receipts != 1 {
				t.Fatal("erasure lost body/hash/metadata boundary", len(body), hash, receipts, err)
			}
		})
	}
}

func TestAdaptiveOpenedEvidenceCurrentBeforeReleaseLifecycleIntegration(t *testing.T) {
	for _, mutation := range []string{"correction", "deletion", "access_revision", "constraints", "stopped"} {
		t.Run(mutation, func(t *testing.T) {
			f, store, answer := actualAdaptiveOpenedEvidence(t)
			raw, _ := json.Marshal(answer)
			if _, err := store.Validate(f.owned, f.call.RunId, string(raw)); err != nil {
				t.Fatal("positive control", err)
			}
			src := f.args.Sources[0]
			var err error
			switch mutation {
			case "correction":
				_, err = f.f.sources.Correct(f.f.owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Corrected evidence"})
			case "deletion":
				_, err = f.f.sources.Delete(f.f.owner, src.ID, "delete", src.Revision)
			case "access_revision":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, src.ID)
			case "constraints":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.f.parent.TaskID)
			case "stopped":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='cancelled',current_state='cancelled' WHERE run_id=$1`, f.call.RunId)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.Validate(f.owned, f.call.RunId, string(raw)); err == nil {
				t.Fatal("late evidence release accepted")
			}
			var receipts int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_retrieval_opened`).Scan(&receipts); err != nil || receipts != 1 {
				t.Fatal("historical metadata lost", receipts, err)
			}
		})
	}
}

func TestAdaptiveIndependentPlanCannotReadEvenWithValidBrokerDispatchIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	store, err := adaptive.NewStore(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Init(f.owned, adaptive.Classify("Вычисли 2+2")); err != nil {
		t.Fatal(err)
	}
	prepareMemoryReadCheckpoint(t, f)
	service, err := NewMemoryReads(f.f.pool, f.f.sources, AdaptiveReadOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	h, err := readcontrol.NewHandler("guarded-request-private-process-fixture-token", service)
	if err != nil {
		t.Fatal(err)
	}
	process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+readcontrol.Open)
	result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err == nil && result.GetStatus() == "completed" {
		t.Fatal("independent optional read published", result)
	}
	var calls int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT calls FROM memory_retrieval_plans WHERE run_id=$1`, f.call.RunId).Scan(&calls); err != nil || calls != 0 {
		t.Fatal("independent read spent", calls, err)
	}
}
