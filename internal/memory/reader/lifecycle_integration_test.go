package reader

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
)

func TestMemoryReadLifecycleAttackCorpusIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "reader-lifecycle"})
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			key, runID := "reader:"+attack.ID, "read-"+attack.ID
			if _, err := pool.Exec(owner, `WITH s AS(INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web') RETURNING session_key),r AS(INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) SELECT $2,session_key,$2,'model_running','model_running','mode_0','fixture',clock_timestamp(),clock_timestamp() FROM s RETURNING run_id,session_key) INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) SELECT session_key,run_id,run_id||'-lease',1,clock_timestamp()+INTERVAL '1 hour' FROM r`, key, runID); err != nil {
				t.Fatal(err)
			}
			live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: key, RunID: runID, LeaseID: runID + "-lease", FenceEpoch: 1})
			src, err := sources.Put(owner, catalog.PutSource{ExternalKey: attack.ID, Kind: "document", ScopeType: "user", ScopeID: "owner", Title: attack.ID, Body: attack.Body, Origin: content.Origin{Kind: content.Kind(attack.OriginKind), SourceRefs: []string{attack.ID}, Sensitivity: content.Private}})
			if err != nil {
				t.Fatal(err)
			}
			read := func(tool string, args readcontrol.Args) readcontrol.Result {
				t.Helper()
				tx, err := pool.Begin(live)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				out, err := (Reader{Sources: sources}).ReadTx(live, tx, tool, args)
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(live); err != nil {
					t.Fatal(err)
				}
				return out
			}
			search := read(readcontrol.Search, readcontrol.Args{Query: attack.ID, Limit: 5})
			if search.Status != "ok" || search.Trust != "DATA" || len(search.Items) != 1 || search.Items[0].Source.ID != src.ID {
				t.Fatal("attack not discoverable as authorized DATA", search.Status, len(search.Items))
			}
			ref := search.Items[0].Source
			args := readcontrol.Args{SourceID: ref.ID, Revision: ref.Revision, AccessRevision: ref.AccessRevision, Hash: ref.Hash, MaxChars: 4096}
			opened := read(readcontrol.Open, args)
			if opened.Status != "ok" || opened.Trust != "DATA" || len(opened.Items) != 1 || opened.Items[0].Text != attack.Body || opened.Items[0].Freshness != "current_revision_unverified_fact" {
				t.Fatal("read changed attack into instructions/verified facts", opened.Status)
			}
			var pins int
			if err = pool.QueryRow(owner, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND source_id=$2 AND source_revision=$3 AND access_revision=$4 AND content_hash=$5`, runID, src.ID, src.Revision, src.AccessRevision, src.Hash).Scan(&pins); err != nil || pins != 1 {
				t.Fatal("read lost exact current source pin", pins, err)
			}
			corrected, err := sources.Correct(owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Corrected independent DATA"})
			if err != nil {
				t.Fatal(err)
			}
			if out := read(readcontrol.Open, args); out.Status != "stale" || len(out.Items) != 0 {
				t.Fatal("old attack handle survived correction", out.Status)
			}
			if _, err = sources.Delete(owner, src.ID, "delete", corrected.Revision); err != nil {
				t.Fatal(err)
			}
			if out := read(readcontrol.Open, args); out.Status != "denied" || len(out.Items) != 0 {
				t.Fatal("old attack handle survived forget", out.Status)
			}
			if _, err = catalog.New(pool, manifest).Get(owner, src.ID); !errors.Is(err, catalog.ErrDenied) {
				t.Fatal("reader restart recovered forgotten source", err)
			}
		})
	}
	var claims, grants, mandates int
	if err := pool.QueryRow(owner, `SELECT (SELECT count(*) FROM memory_claims),(SELECT count(*) FROM action_authorizations),(SELECT count(*) FROM action_mandates)`).Scan(&claims, &grants, &mandates); err != nil || claims != 0 || grants != 0 || mandates != 0 {
		t.Fatal("read consumer created facts or authority", claims, grants, mandates, err)
	}
}
