package completion

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// The real coordinator, dispatch journal and deletion controller share a two-
// connection pool. They must serialize without borrowing a third connection.
func TestSourceDeletionVersusAtomicResultAndDispatchIntegration(t *testing.T) {
	for _, order := range []string{"delete_first", "result_first", "simultaneous"} {
		t.Run(order, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			own := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
			if e := manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			sources := catalog.New(pool, manifest)
			_, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','running','mode_1','finalizing','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = pool.Exec(ctx, `UPDATE runs SET lease_id='lease' WHERE run_id='run'`); e != nil {
				t.Fatal(e)
			}
			live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
			src, e := sources.Put(own, catalog.PutSource{ExternalKey: "document", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private input"})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = sources.Get(live, src.ID); e != nil {
				t.Fatal(e)
			}
			c := New(pool)
			c.DataGuard = sources.CheckRunSourcesTx
			q := Request{RunID: "run", SessionKey: "web:owner", Content: "derived private answer"}
			var saveErr, deleteErr error
			save := func() { _, saveErr = c.Save(live, q) }
			remove := func() { _, deleteErr = sources.Delete(own, src.ID, "delete", src.Revision) }
			switch order {
			case "delete_first":
				remove()
				save()
			case "result_first":
				save()
				remove()
			default:
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); save() }()
				go func() { defer wg.Done(); remove() }()
				wg.Wait()
			}
			if deleteErr != nil {
				t.Fatal(deleteErr)
			}
			if order == "delete_first" && saveErr == nil {
				t.Fatal("deleted source published")
			}
			if order == "result_first" && saveErr != nil {
				t.Fatal(saveErr)
			}
			var count int
			if e = pool.QueryRow(ctx, `SELECT count(*) FROM run_final_results WHERE content<>''`).Scan(&count); e != nil || count != 0 {
				t.Fatal("private result survived", count, e)
			}
			if e = pool.QueryRow(ctx, `SELECT count(*) FROM delivery_outbox WHERE content<>''`).Scan(&count); e != nil || count != 0 {
				t.Fatal("private notification survived", count, e)
			}
			if saveErr == nil {
				r, e := c.GetByRun(ctx, "run")
				if e != nil || !r.PrivacyRedacted || r.ContentHash == "" || r.Verdict != "unverified" {
					t.Fatal("immutable evidence lost", r, e)
				}
			}
			// Even a still-unused exact grant cannot cross the effect boundary after
			// source deletion; the journal remains allowed, never executing.
			a := actionauth.NewPostgresStore(pool)
			a.SetRunDataGuard(sources.CheckRunSourcesTx)
			binding := actionauth.ActionBinding{ActionID: "action", ActionHash: "hash", RunID: "run", ToolCallID: "call", ToolName: "http.request", OwnerID: "owner", Effect: actionauth.EffectCommit}
			if e = a.PutIntent(ctx, actionauth.Intent{Binding: binding, ExpiresAt: time.Now().Add(time.Minute), EffectID: "effect", EffectFingerprint: "fingerprint", EffectPolicy: actionauth.EffectPolicy{RetrySafety: "never", VerificationMethod: "none"}}); e != nil {
				t.Fatal(e)
			}
			if _, e = a.PutGrant(ctx, actionauth.Grant{ID: "grant", Binding: binding, LeaseID: "lease", LeaseEpoch: 1, ExpiresAt: time.Now().Add(time.Minute)}); e != nil {
				t.Fatal(e)
			}
			if e = a.ClaimGrant(ctx, "grant"); e == nil {
				t.Fatal("deleted data dispatched")
			}
			var state string
			if e = pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id='effect'`).Scan(&state); e != nil || state != "allowed" {
				t.Fatal(state, e)
			}
		})
	}
}

func TestSourceDeletionVersusLiveDispatchClaimIntegration(t *testing.T) {
	for _, order := range []string{"delete_first", "claim_first", "simultaneous"} {
		t.Run(order, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			own := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
			if e := manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			sources := catalog.New(pool, manifest)
			if _, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','running','mode_1','tool_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); e != nil {
				t.Fatal(e)
			}
			live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
			src, e := sources.Put(own, catalog.PutSource{ExternalKey: "document", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private data"})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = sources.Get(live, src.ID); e != nil {
				t.Fatal(e)
			}
			a := actionauth.NewPostgresStore(pool)
			a.SetRunDataGuard(sources.CheckRunSourcesTx)
			binding := actionauth.ActionBinding{ActionID: "action", ActionHash: "hash", RunID: "run", ToolCallID: "call", ToolName: "http.request", OwnerID: "owner", Effect: actionauth.EffectCommit}
			if e = a.PutIntent(ctx, actionauth.Intent{Binding: binding, ExpiresAt: time.Now().Add(time.Minute), EffectID: "effect", EffectFingerprint: "fingerprint", EffectPolicy: actionauth.EffectPolicy{RetrySafety: "never", VerificationMethod: "none"}}); e != nil {
				t.Fatal(e)
			}
			if _, e = a.PutGrant(ctx, actionauth.Grant{ID: "grant", Binding: binding, LeaseID: "lease", LeaseEpoch: 1, ExpiresAt: time.Now().Add(time.Minute)}); e != nil {
				t.Fatal(e)
			}
			var claimErr, deleteErr error
			claim := func() { claimErr = a.ClaimGrant(ctx, "grant") }
			remove := func() { _, deleteErr = sources.Delete(own, src.ID, "delete", src.Revision) }
			switch order {
			case "delete_first":
				remove()
				claim()
			case "claim_first":
				claim()
				remove()
			default:
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); claim() }()
				go func() { defer wg.Done(); remove() }()
				wg.Wait()
			}
			if deleteErr != nil {
				t.Fatal(deleteErr)
			}
			if order == "delete_first" && claimErr == nil {
				t.Fatal("deleted source dispatched")
			}
			if order == "claim_first" && claimErr != nil {
				t.Fatal(claimErr)
			}
			var state string
			if e = pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id='effect'`).Scan(&state); e != nil {
				t.Fatal(e)
			}
			want := "allowed"
			if claimErr == nil {
				want = "executing"
			}
			if state != want {
				t.Fatal("effect boundary history changed", state, want)
			}
			if e = a.ClaimGrant(ctx, "grant"); e == nil {
				t.Fatal("deleted/spent action was dispatched again")
			}
		})
	}
}
