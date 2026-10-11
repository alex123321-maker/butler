package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestMemoryCandidateInputExpiresDuringDedupWaitIntegration(t *testing.T) {
	f := newToolControlDispatchFixture(t, nil, false, true)
	inputs := durableinput.NewStore(f.f.pool)
	checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
	if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
		t.Fatal(err)
	}
	src := f.args.Sources[0]
	tx, err := f.f.pool.Begin(f.owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.sources.AccessTx(f.owned, tx, []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, "memory_read"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.owned); err != nil {
		t.Fatal(err)
	}
	f.claim(t)
	service, err := NewMemoryCandidates(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.f.ctx, 10*time.Second)
	defer cancel()
	if _, err = f.f.pool.Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE input_id=$1`, f.inputClaim.InputID); err != nil {
		t.Fatal(err)
	}
	blocker, err := f.f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(ctx, `INSERT INTO memory_extraction_dedup(owner_id,source_id,source_revision,dedup_key,request_hash,claim_id) VALUES($1,$2,$3,$4,$5,'uncommitted-blocker')`, f.f.parent.OwnerID, src.ID, src.Revision, f.prepared.EffectID, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	e := candidatecontrol.Envelope{RunID: f.envelope.RunID, ToolCallID: f.envelope.ToolCallID, EffectID: f.envelope.EffectID, ArgsJSON: f.envelope.ArgsJSON, ContractHash: f.envelope.ContractHash}
	result := make(chan error, 1)
	go func() { _, err := service.ProposeDispatched(ctx, e); result <- err }()
	for {
		var waiting bool
		// pg_stat_activity's query text can be cached by this long transaction.
		// Refresh it and prove that THIS blocker owns the lock being waited on.
		if _, err = blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err = blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND pg_backend_pid()=ANY(pg_blocking_pids(pid)) AND query LIKE '%INSERT INTO memory_extraction_dedup%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-result:
			t.Fatal("owning publication did not wait on dedup", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		var expired bool
		if err = blocker.QueryRow(ctx, `SELECT claim_expires_at<=clock_timestamp() FROM accepted_inputs WHERE input_id=$1`, f.inputClaim.InputID).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil {
		t.Fatal("expired input claim published candidate after wait")
	}
	var claims, dedup int
	if err = f.f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_claims),(SELECT count(*) FROM memory_extraction_dedup)`).Scan(&claims, &dedup); err != nil || claims != 0 || dedup != 0 {
		t.Fatal("denied late publication retained candidate/dedup", claims, dedup, err)
	}
}
