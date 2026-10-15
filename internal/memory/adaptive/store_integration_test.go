package adaptive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestAdaptiveDurableReadBoundsSurviveReconstructionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, m)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "adaptive-fixture"})
	if _, err := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('adaptive','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('adaptive-run','adaptive','input','model_running','model_running','mode_0','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('adaptive','adaptive-run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	ctx := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "adaptive", RunID: "adaptive-run", LeaseID: "lease", FenceEpoch: 1})
	s, err := NewStore(pool, c)
	if err != nil {
		t.Fatal(err)
	}
	plan := Classify("Вспомни наши записи")
	if err = s.Init(ctx, plan); err != nil {
		t.Fatal(err)
	}
	search := readcontrol.Args{Query: "alpha", Strategy: "semantic_hybrid", Limit: 4}
	reserve := func(i int, tool string, a readcontrol.Args) error {
		_, err := s.Reserve(ctx, readcontrol.Envelope{RunID: "adaptive-run", ToolCallID: fmt.Sprint("call-", i), EffectID: fmt.Sprint("effect-", i), Tool: tool, ArgsJSON: "{}"}, a)
		return err
	}
	if err = reserve(0, readcontrol.Search, search); err != nil {
		t.Fatal(err)
	}
	if err = reserve(1, readcontrol.Search, search); !errors.Is(err, ErrBound) {
		t.Fatal("second recall search admitted", err)
	}
	for i := 1; i < 4; i++ {
		if err = reserve(i, readcontrol.Open, readcontrol.Args{}); err != nil {
			t.Fatal(err)
		}
	}
	s, err = NewStore(pool, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Init(ctx, plan); err != nil {
		t.Fatal("same immutable restart", err)
	}
	if err = reserve(4, readcontrol.Open, readcontrol.Args{}); !errors.Is(err, ErrBound) {
		t.Fatal("restart reset calls", err)
	}
	if err = s.CheckProposal(ctx, "adaptive-run", readcontrol.Open, `{"source_id":"source_neutral","source_revision":1,"access_revision":1,"content_hash":"`+strings.Repeat("a", 64)+`"}`); !errors.Is(err, ErrBound) {
		t.Fatal("exhausted proposal reached dispatch", err)
	}
	var calls, searches, operations int
	if err = pool.QueryRow(ctx, `SELECT calls,searches,(SELECT count(*) FROM memory_retrieval_operations) FROM memory_retrieval_plans WHERE run_id='adaptive-run'`).Scan(&calls, &searches, &operations); err != nil || calls != 4 || searches != 1 || operations != 4 {
		t.Fatal(calls, searches, operations, err)
	}
	expanded := plan
	expanded.MaxCalls = 8
	if err = s.Init(ctx, expanded); !errors.Is(err, ErrDenied) {
		t.Fatal("plan rebased", err)
	}
	if _, err = pool.Exec(owner, `UPDATE memory_retrieval_plans SET calls=0,deadline=clock_timestamp()-INTERVAL '1 second' WHERE run_id='adaptive-run'`); err != nil {
		t.Fatal(err)
	}
	if err = reserve(5, readcontrol.Open, readcontrol.Args{}); !errors.Is(err, ErrBound) {
		t.Fatal("elapsed window restarted", err)
	}
	// A new ownership epoch cannot reuse the old context, irrespective of the
	// durable plan and remaining byte/call counters.
	if _, err = pool.Exec(owner, `UPDATE session_ownership SET fence_epoch=2 WHERE session_key='adaptive'`); err != nil {
		t.Fatal(err)
	}
	if err = reserve(6, readcontrol.Open, readcontrol.Args{}); err == nil {
		t.Fatal("old lease reused")
	}
}
