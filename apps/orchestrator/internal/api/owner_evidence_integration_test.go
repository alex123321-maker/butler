package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestOwnerTaskEvidenceHTTPKeepsResultsEffectsDeliveryAndReplayDistinctIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	sessions := session.NewPostgresRepository(pool)
	_, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "owner-evidence", UserID: "owner", Channel: "telegram", MetadataJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	store := task.NewStore(pool)
	job, err := store.Create(ctx, task.CreateRequest{SessionKey: "owner-evidence", OwnerID: "owner", Goal: "Owner checks evidence", CompletionCriteria: []string{"original observed evidence"}, Source: map[string]any{"private_body": "PRIVATE_EVIDENCE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	runs := run.NewPostgresRepository(pool)
	_, err = runs.CreateRun(ctx, run.Record{RunID: "owner-evidence-run", SessionKey: "owner-evidence", InputEventID: "fixture", Status: "running", CurrentState: "finalizing", AutonomyMode: "mode_1", ModelProvider: "fixture", MetadataJSON: `{}`, StartedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AttachRun(ctx, job.TaskID, 1, "owner-evidence-run", job.SessionKey); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: job.SessionKey, RunID: "owner-evidence-run", OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: job.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if _, err = pool.Exec(ctx, `UPDATE runs SET lease_id=$1 WHERE run_id='owner-evidence-run'`, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	saved, err := completion.New(pool).Save(owned, completion.Request{RunID: "owner-evidence-run", SessionKey: job.SessionKey, TaskID: job.TaskID, TaskRevision: 1, ConstraintsRevision: 1, Content: strings.Repeat("PRIVATE_EVIDENCE_BODY", 300), AnswerState: "final"})
	if err != nil {
		t.Fatal(err)
	}
	// One accepted chunk does not hide an unknown acknowledgement for another.
	if _, err = pool.Exec(ctx, `UPDATE delivery_outbox SET status=CASE WHEN chunk_index=0 THEN 'sent' ELSE 'delivery_unknown' END WHERE run_id='owner-evidence-run'`); err != nil {
		t.Fatal(err)
	}
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "verification"})
	verified, err := store.SaveVerifiedResult(actor, task.Result{TaskID: job.TaskID, TaskRevision: 1, RunID: saved.RunID, Summary: "PRIVATE_EVIDENCE_BODY", EvidenceRefs: []string{"artifact:" + saved.ArtifactID, "source:visible-source", "/srv/private/import.json", "https://private.invalid/?token=PRIVATE_EVIDENCE_BODY"}, VerifiedCriteria: job.CompletionCriteria, Verifier: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	newServer := func() *TaskControlServer {
		s := NewTaskControlServer(task.NewStore(pool))
		s.SetOwnerControlAuth(NewOwnerControlAuth("fixture-owner-token-000000000000000", "owner"))
		s.SetControls(task.NewController(store, leases.Authority(), leases, runs, approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool)))
		return s
	}
	s := newServer()
	server := httptest.NewServer(s.HandleItem("/api/v3/tasks/"))
	defer server.Close()
	read := func() task.OwnerOverview {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v3/tasks/"+job.TaskID+"/observations", nil)
		req.Header.Set("Authorization", "Bearer fixture-owner-token-000000000000000")
		res, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 || bytes.Contains(raw, []byte("PRIVATE_EVIDENCE_BODY")) || bytes.Contains(raw, []byte("/srv/private")) {
			t.Fatalf("unsafe/unavailable projection: status%d", res.StatusCode)
		}
		var view task.OwnerOverview
		if json.Unmarshal(raw, &view) != nil {
			t.Fatal("invalid owner projection")
		}
		return view
	}
	v := read()
	if v.Status != "active" || v.CompletionResultID != "" || len(v.Attempts) != 1 || v.Attempts[0].State != "completed" || v.Attempts[0].DeliveryState != "delivery_unknown" {
		t.Fatalf("run/task/delivery conflated: %#v", v)
	}
	found := false
	for _, result := range v.Results {
		if result.ResultID == verified.ResultID {
			found = result.Verdict == "verified" && result.CurrentRevision && len(result.Evidence) == 2 && result.OmittedReferences == 2
		}
	}
	if !found {
		t.Fatal("verified typed evidence refs unavailable or private refs copied")
	}
	// The actual HTTP handler commits, then the first connection loses its ACK.
	server.Close()
	var first atomic.Bool
	first.Store(true)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && first.CompareAndSwap(true, false) {
			recorded := httptest.NewRecorder()
			s.HandleItem("/api/v3/tasks/").ServeHTTP(recorded, r)
			if recorded.Code != 200 {
				t.Errorf("control did not commit: %d %s", recorded.Code, recorded.Body.String())
			}
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			_ = conn.Close()
			return
		}
		newServer().HandleItem("/api/v3/tasks/").ServeHTTP(w, r)
	}))
	defer server.Close()
	control := func(instruction string) (*http.Response, error) {
		raw, _ := json.Marshal(task.ControlRequest{Kind: task.ControlAmend, ExpectedRevision: 1, Instruction: instruction})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v3/tasks/"+job.TaskID+"/control", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer fixture-owner-token-000000000000000")
		req.Header.Set("X-Butler-Delivery-ID", "owner-lost-ack")
		return server.Client().Do(req)
	}
	if response, e := control("Only prepare a correction"); e == nil {
		response.Body.Close()
		t.Fatal("lost ACK unexpectedly visible")
	}
	response, err := control("Only prepare a correction")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("saved receipt replay failed: %d %s", response.StatusCode, raw)
	}
	record, err := store.Get(ctx, job.TaskID)
	if err != nil || record.Revision != 2 {
		t.Fatal("lost ACK repeated amendment")
	}
	v = read()
	if v.Revision != 2 {
		t.Fatal("new store did not observe current revision")
	}
	for _, result := range v.Results {
		if result.ResultID == verified.ResultID && result.CurrentRevision {
			t.Fatal("old verification presented as current after amendment")
		}
	}
	response, err = control("Changed content with old delivery")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("changed delivery replay accepted")
	}
}
