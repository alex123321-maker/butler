package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestCurrentProjectRuleIsScopedAndOldDocumentRemainsCandidateIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "scoped-rules"})
	_, e := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('session','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('task-a','session','owner','project A');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,task_id,task_revision) VALUES('run','session','input','running','mode_1','model_running','test',clock_timestamp(),clock_timestamp(),'task-a',1);INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('session','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if e != nil {
		t.Fatal(e)
	}
	confirmed := func(project, value string) catalog.Source {
		src, e := c.Put(owner, catalog.PutSource{ExternalKey: project, Kind: "owner_statement", ScopeType: "project", ScopeID: project, Body: value, Title: project})
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(value)
		candidate, e := c.Propose(owner, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "language", Value: raw, Summary: value, Confidence: 1})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = c.Decide(owner, catalog.Decision{ClaimID: candidate.ID}); e != nil {
			t.Fatal(e)
		}
		return src
	}
	a := confirmed("owner:project-a", "Пиши только по-русски для проекта A")
	confirmed("owner:project-b", "NurDeutschProjektB")
	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old, e := c.Put(owner, catalog.PutSource{ExternalKey: "old-file-today", Kind: "document", ScopeType: "project", ScopeID: a.ScopeID, Author: "Old author", EventTime: &oldTime, Title: "Old policy", Body: "EnglishOnlyOldDocument"})
	if e != nil || old.EventTime == nil || !old.EventTime.Equal(oldTime) || old.ExtractedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal(old, e)
	}
	claim, e := c.Propose(owner, catalog.ClaimRequest{SourceID: old.ID, SourceRevision: old.Revision, Key: "language", Value: json.RawMessage(`"en"`), Summary: old.Body, Confidence: 1})
	if e != nil || claim.Status != "candidate" {
		t.Fatal(claim, e)
	}
	if e = c.GrantSource(owner, "task-a", a.ID, "memory_read", 1); e != nil {
		t.Fatal(e)
	}
	current := ownership.WithToken(context.Background(), nil, ownership.Token{RunID: "run", SessionKey: "session", LeaseID: "lease", FenceEpoch: 1})
	service := memoryservice.New(memoryservice.Config{ProfileStore: profileStoreAdapter{store: profile.NewStore(pool), sources: c}, ScopeResolver: func(ctx context.Context) ([]memoryservice.Scope, error) {
		scopes, e := c.ExtraScopes(ctx, "memory_read")
		out := make([]memoryservice.Scope, len(scopes))
		for i, s := range scopes {
			out[i] = memoryservice.Scope{Type: s.Type, ID: s.ID}
		}
		return out, e
	}})
	bundle, e := service.BuildBundle(current, memoryservice.BundleRequest{SessionKey: "session", UserID: "owner"})
	if e != nil || !strings.Contains(bundle.Prompt, "Пиши только по-русски для проекта A") || strings.Contains(bundle.Prompt, "NurDeutschProjektB") || strings.Contains(bundle.Prompt, "EnglishOnlyOldDocument") {
		t.Fatal("scope or candidate isolation", e, bundle.Prompt)
	}
	if _, e = pool.Exec(owner, `UPDATE tasks SET revision=revision+1 WHERE task_id='task-a'`); e != nil {
		t.Fatal(e)
	}
	if _, e = service.BuildBundle(current, memoryservice.BundleRequest{SessionKey: "session", UserID: "owner"}); e == nil {
		t.Fatal("stale project scope retained after amendment")
	}
}
