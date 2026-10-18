package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
)

func TestTemporalOneOffExpiryPreservesPermanentProfileInOrdinaryExecuteIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, m)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "temporal-context"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('temporal-a','owner','web'),('temporal-b','owner','web')`); err != nil {
		t.Fatal(err)
	}
	seed := func(scope, id, key, body string, class catalog.FreshnessClass, to *time.Time) catalog.Claim {
		t.Helper()
		src, err := c.Put(ctx, catalog.PutSource{ExternalKey: key, Kind: "owner_statement", ScopeType: scope, ScopeID: id, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		claim, err := c.Propose(ctx, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: "language", Value: json.RawMessage(`"owner preference"`), Summary: body})
		if err != nil {
			t.Fatal(err)
		}
		claim, err = c.Decide(ctx, catalog.Decision{ClaimID: claim.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityInterval, ValidTo: to, Class: class}); err != nil {
			t.Fatal(err)
		}
		return claim
	}
	const permanent = "Permanent preference: Russian explanations."
	const exception = "One-off session exception: English explanations."
	stable := seed("user", "owner", "stable", permanent, catalog.FreshnessStablePreference, nil)
	future := time.Now().Add(time.Hour).UTC()
	oneoff := seed("session", "temporal-a", "one-off", exception, catalog.FreshnessWorkStatus, &future)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.Policy.Models["antigravity:temporal-fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p := &scopedCandidateProvider{}
	serial := 0
	run := func(scope string, wantException, annotate bool) {
		t.Helper()
		serial++
		id := fmt.Sprintf("temporal-%d", serial)
		executor := flow.NewService(session.NewPostgresRepository(pool), session.NewPostgresLeaseManager(pool), runservice.NewService(runservice.NewPostgresRepository(pool), nil), transcript.NewStore(pool), p, flow.Config{ProviderName: p.Name(), ModelName: "temporal-fixture", OwnerID: "owner", LeaseTTL: 120, Budgets: b, MemorySources: c, ProfileStore: profileStoreAdapter{store: profile.NewStore(pool), sources: c, temporal: annotate}, Completion: completion.New(pool)}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		payload, _ := json.Marshal(map[string]string{"text": "Use current accepted language preferences.", "user_id": "owner"})
		if _, err := executor.Execute(ctx, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: scope, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}); err != nil {
			t.Fatal("ordinary temporal execution", err)
		}
		raw, _ := json.Marshal(p.last.InputItems)
		if !strings.Contains(string(raw), permanent) || strings.Contains(string(raw), exception) != wantException {
			t.Fatal("wrong scoped current context", scope, wantException)
		}
		if strings.Contains(string(raw), "[Stored temporal evidence;") != annotate {
			t.Fatal("temporal annotation did not follow feature state")
		}
	}
	run("temporal-a", true, true)
	run("temporal-b", false, true)
	run("temporal-a", true, false)
	past := time.Now().Add(-time.Hour).UTC()
	if _, err := c.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: oneoff.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: catalog.ValidityInterval, ValidTo: &past, Class: catalog.FreshnessWorkStatus}); err != nil {
		t.Fatal(err)
	}
	run("temporal-a", false, true)
	// Disabling new temporal consumers must not revive already expired DATA.
	run("temporal-a", false, false)
	before := past.Add(-time.Hour)
	history, err := c.ReadAcceptedFacts(ctx, []string{oneoff.ID}, catalog.TemporalQuery{ValidAt: &before})
	if err != nil || len(history.Items) != 1 || history.Items[0].Freshness.Status != "expired" {
		t.Fatal("one-off history erased", history, err)
	}
	current, err := c.ReadAcceptedFacts(ctx, []string{stable.ID}, catalog.TemporalQuery{})
	if err != nil || len(current.Items) != 1 || current.Items[0].Freshness.Status != "stable" {
		t.Fatal("exception expiry rewrote permanent owner head", current, err)
	}
}

func TestTemporalEpisodeExpirySurvivesDisabledAnnotationsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	anchor := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := anchor.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, anchor)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "episode-expiry"})
	src, err := c.Put(ctx, catalog.PutSource{ExternalKey: "episode-expiry", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Temporary deployment observation."})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := c.Propose(ctx, catalog.ClaimRequest{MemoryType: "episodic", SourceID: src.ID, SourceRevision: 1, Key: "episode.temporary", Summary: src.Body, Value: json.RawMessage(`"reported observation"`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Decide(ctx, catalog.Decision{ClaimID: claim.ID}); err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, embeddings.VectorDimensions())
	vector[0] = 1
	encoded, _ := json.Marshal(vector)
	if _, err = pool.Exec(ctx, `UPDATE memory_episodes SET embedding=$1::vector WHERE source_id=$2`, string(encoded), src.ID); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UTC()
	if _, err = c.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityInterval, ValidTo: &future, Class: catalog.FreshnessCompletedEpisode}); err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		for _, annotate := range []bool{false, true} {
			a := episodicStoreAdapter{store: episodic.NewStore(pool), sources: c, temporal: annotate}
			found, err := a.FindBySummary(ctx, "user", "owner", src.Body)
			if err != nil || len(found) != want {
				t.Fatalf("exact episode selection annotation=%t count=%d want=%d error=%v", annotate, len(found), want, err)
			}
			nearest, err := a.Search(ctx, "user", "owner", vector, 5)
			if err != nil || len(nearest) != want {
				t.Fatalf("semantic episode selection annotation=%t count=%d want=%d error=%v", annotate, len(nearest), want, err)
			}
			if want == 1 && strings.Contains(found[0].EpisodeSummary(), "[Stored temporal evidence;") != annotate {
				t.Fatal("episode annotation did not follow feature state")
			}
		}
	}
	check(1)
	past := time.Now().Add(-time.Hour).UTC()
	if _, err = c.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, ExpectedRevision: 1, Validity: catalog.ValidityInterval, ValidTo: &past, Class: catalog.FreshnessCompletedEpisode}); err != nil {
		t.Fatal(err)
	}
	check(0)
	var status, confirmation string
	if err := pool.QueryRow(ctx, `SELECT status,confirmation_state FROM memory_episodes WHERE source_id=$1`, src.ID).Scan(&status, &confirmation); err != nil || status != "active" || confirmation != "confirmed" {
		t.Fatal("temporal expiry changed durable episode lifecycle", status, confirmation, err)
	}
}
