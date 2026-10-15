package reader

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestAdaptiveReportedEventFilterAndSemanticCandidateAuthorityIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, m)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "adaptive-reader"})
	if _, err := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('adaptive','owner','web'),('foreign','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('adaptive-run','adaptive','input','model_running','model_running','mode_0','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('adaptive','adaptive-run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	ctx := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "adaptive", RunID: "adaptive-run", LeaseID: "lease", FenceEpoch: 1})
	put := func(key, scope, id, date string) catalog.Source {
		t.Helper()
		var at *time.Time
		if date != "" {
			v, err := time.Parse(time.RFC3339, date)
			if err != nil {
				t.Fatal(err)
			}
			at = &v
		}
		src, err := c.Put(owner, catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: scope, ScopeID: id, Title: "needle", Body: "needle evidence", EventTime: at})
		if err != nil {
			t.Fatal(err)
		}
		return src
	}
	in := put("in", "user", "owner", "2025-09-01T00:00:00Z")
	_ = put("until", "user", "owner", "2025-10-01T00:00:00Z")
	_ = put("unknown", "user", "owner", "")
	foreign := put("foreign", "session", "foreign", "2025-09-01T00:00:00Z")
	read := func(r Reader, a readcontrol.Args) readcontrol.Result {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		out, err := r.ReadTx(ctx, tx, readcontrol.Search, a)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := readcontrol.Args{Query: "needle", Limit: 10, Strategy: "catalog_exact", EventFrom: "2025-09-01T00:00:00Z", EventUntil: "2025-10-01T00:00:00Z"}
	out := read(Reader{Sources: c}, a)
	if out.Status != "ok" || len(out.Items) != 1 || out.Items[0].Source.ID != in.ID || out.Items[0].ReadRef != "" || len(out.Diagnostics) != 2 {
		t.Fatal("time/scope filter", out)
	}
	var candidates []SemanticCandidate
	for _, src := range []catalog.Source{in, foreign} {
		if _, err := c.PublishFragments(owner, catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}, []catalog.Fragment{{Key: "fragment", Text: "needle chunk", Locator: json.RawMessage(`{}`)}}); err != nil {
			t.Fatal(err)
		}
		var next SemanticCandidate
		if err := pool.QueryRow(owner, `SELECT c.id,c.updated_at FROM memory_chunks c JOIN memory_object_sources o ON o.object_id=c.id AND o.memory_type='chunk' WHERE o.source_id=$1`, src.ID).Scan(&next.ID, &next.Revision); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, next)
	}
	a.Strategy = "semantic_hybrid"
	a.Limit = 4
	out = read(Reader{Sources: c, SemanticCandidates: candidates}, a)
	if out.Status != "ok" || len(out.Items) != 1 || out.Items[0].Source.ID != in.ID || !out.Truncated || out.Items[0].ReadRef != "" {
		t.Fatal("semantic candidate bypassed source authority", out)
	}
	candidates[0].Revision = candidates[0].Revision.Add(-time.Second)
	out = read(Reader{Sources: c, SemanticCandidates: candidates[:1]}, a)
	if out.Status != "empty" || len(out.Items) != 0 || !out.Truncated {
		t.Fatal("stale candidate accepted", out)
	}
}
