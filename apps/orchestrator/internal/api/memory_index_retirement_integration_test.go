package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerIndexRetirementAPIWorksWithoutModelRuntimeIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, m)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "prepare"})
	src, err := cat.Put(owner, catalog.PutSource{ExternalKey: "source", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Original retained"})
	if err != nil {
		t.Fatal(err)
	}
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j := jobs.New(pool, cat, b, nil)
	runtime := &indexing.Runtime{Catalog: cat, Jobs: j, Space: semantic.Space{ProfileID: "fixture", Model: "google/embeddinggemma-2", Revision: strings.Repeat("a", 40), Preprocessing: "embeddinggemma2-retrieval-v1", RuntimeVersion: "fixture-v1", ChunkVersion: "fixture-v1", Dimensions: 768}}
	receipt, err := runtime.Enqueue(owner, indexing.Request{ScopeType: "user", ScopeID: "owner", MaxChunks: 1, Job: jobs.Request{ID: "owner-job", Kind: "index", Purpose: "memory_read", Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 1000, ActiveMillis: 1000, ProviderInputBytes: 1000}, Deadline: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := j.ClaimKind(context.Background(), "index")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err = j.Finish(owner, claimed, "no_model_requested", false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("p", 40)
	server := &MemoryCatalogServer{Store: cat, Auth: NewOwnerControlAuth(token, "owner")} // Indexing remains nil.
	request := func(body, bearer, delivery string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v4/memory/index-retire", strings.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		req.Header.Set("X-Butler-Delivery-ID", delivery)
		w := httptest.NewRecorder()
		server.Handle().ServeHTTP(w, req)
		return w
	}
	body, _ := json.Marshal(semantic.RetireRequest{GenerationID: receipt.GenerationID})
	if w := request(string(body), "", "one"); w.Code != http.StatusUnauthorized {
		t.Fatal("anonymous mutation", w.Code)
	}
	if w := request(string(body), token, ""); w.Code != http.StatusBadRequest {
		t.Fatal("missing delivery accepted", w.Code)
	}
	server.Auth = NewOwnerControlAuth(token, "other")
	if w := request(string(body), token, "foreign"); w.Code != http.StatusForbidden {
		t.Fatal("foreign owner mutation", w.Code, w.Body.String())
	}
	server.Auth = NewOwnerControlAuth(token, "owner")
	if w := request(string(body[:len(body)-1])+`,"owner_id":"other"}`, token, "invalid"); w.Code != http.StatusBadRequest {
		t.Fatal("request chose owner", w.Code)
	}
	w := request(string(body), token, "retire")
	var out semantic.Retirement
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.JobID != claimed.ID || out.AlreadyRetired {
		t.Fatal("maintenance needed model runtime", w.Code, w.Body.String())
	}
	w = request(string(body), token, "retry")
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &out) != nil || !out.AlreadyRetired {
		t.Fatal("retry failed", w.Code, w.Body.String())
	}
	if current, err := cat.Get(owner, src.ID); err != nil || current.Body != src.Body {
		t.Fatal("source erased", err)
	}
}
