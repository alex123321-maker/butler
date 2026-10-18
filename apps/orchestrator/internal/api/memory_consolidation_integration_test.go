package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestConsolidationOwnerHTTPAtomicPrepareRetryAndPrivateListIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, m)
	j := jobs.New(pool, c, budget.NewController(pool, budget.DefaultPolicy()), nil)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "seed"})
	if _, err := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('completed-session','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('terminal-run','completed-session','input','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes')`); err != nil {
		t.Fatal(err)
	}
	src, err := c.Put(owner, catalog.PutSource{ExternalKey: "note", Kind: "document", ScopeType: "session", ScopeID: "completed-session", Body: "Исключение: неудачная проверка не доказывает успех."})
	if err != nil {
		t.Fatal(err)
	}
	server := &MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("t", 40), "owner"), Consolidation: &consolidation.Store{Catalog: c, Jobs: j}, ConsolidationReady: true}
	request := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		if auth {
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 40))
		}
		r.Header.Set("X-Butler-Delivery-ID", "control")
		w := httptest.NewRecorder()
		server.Handle().ServeHTTP(w, r)
		return w
	}
	q := consolidation.PrepareRequest{ViewKey: "review", Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Policy: jobs.ModelPolicyRequest{ID: "owner-prepared", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Contexts: []jobs.ModelContextRef{{SessionKey: "completed-session", OriginalRunID: "terminal-run"}}, Job: jobs.Request{Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ActiveMillis: 10000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}}
	const endpoint = "/api/v4/memory/consolidation"
	for range 2 {
		w := request("POST", endpoint+"/prepare", q, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var inputs, policies int
	if err = pool.QueryRow(owner, `SELECT (SELECT count(*) FROM memory_consolidation_inputs),(SELECT count(*) FROM memory_model_policies)`).Scan(&inputs, &policies); err != nil || inputs != 1 || policies != 1 {
		t.Fatal("HTTP retry orphaned snapshot/policy", inputs, policies, err)
	}
	q.Policy.Contexts = nil
	q.Policy.ID = "unbounded-context"
	if w := request("POST", endpoint+"/prepare", q, true); w.Code != 400 {
		t.Fatal("missing explicit contexts admitted", w.Code)
	}
	if w := request("GET", endpoint, nil, false); w.Code != 401 {
		t.Fatal("private list exposed", w.Code)
	}
	w := request("GET", endpoint, nil, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Исключение") || !strings.Contains(w.Body.String(), "owner-prepared") {
		t.Fatal("content-free owner receipt missing", w.Code, w.Body.String())
	}
	server.ConsolidationReady = false
	if w = request("POST", endpoint+"/prepare", q, true); w.Code != 503 {
		t.Fatal("operator disable ignored", w.Code)
	}
	if w = request("GET", endpoint, nil, true); w.Code != 200 {
		t.Fatal("operational list disabled", w.Code)
	}
}
