package app

import (
	"log/slog"
	"os"
	"strings"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
)

func configureOwnedSemantic(log *slog.Logger, sources *catalog.Store, b *budget.Controller) (*indexing.Runtime, func()) {
	if strings.TrimSpace(os.Getenv("BUTLER_MEMORY_SEMANTIC_ENABLED")) != "true" {
		return nil, nil
	}
	fail := func() (*indexing.Runtime, func()) {
		log.Warn("owned local memory unavailable: pinned service, private token and exact local pricing required")
		return nil, nil
	}
	price, ok := b.Policy.Models["local:google/embeddinggemma-2"]
	if !ok || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return fail()
	}
	target, e := budget.NewLocalTarget("local-models:9090", strings.TrimSpace(os.Getenv("BUTLER_MEMORY_LOCAL_SERVICE_ID")))
	if e != nil {
		return fail()
	}
	backend, closeBackend, e := semantic.NewPinnedBackend(target, strings.TrimSpace(os.Getenv("BUTLER_MEMORY_LOCAL_TOKEN_FILE")))
	if e != nil {
		return fail()
	}
	guard, e := semantic.NewCatalogGuard(sources, "memory_read")
	if e != nil {
		closeBackend()
		return fail()
	}
	live, e := semantic.NewLiveAdmission(b, guard, target)
	if e != nil {
		closeBackend()
		return fail()
	}
	space := semantic.Space{ProfileID: "embeddinggemma2-text-bf16-v1", Model: "google/embeddinggemma-2", Revision: "914f7f89142e33e77833254d9c9b90c3cef7303b", Preprocessing: "embeddinggemma2-retrieval-v1", RuntimeVersion: "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0", ChunkVersion: "structured-go-v1", Dimensions: 768}
	store := semantic.NewStore(sources.Pool())
	store.Guard = guard
	reader := &semantic.HybridReader{Store: store, Adapter: &semantic.Adapter{Backend: backend, Space: space, Admission: live}, Rerank: false, Log: log}
	ownerSearch, e := semantic.NewOwnerSearch(sources, b, backend, target, space)
	if e != nil {
		closeBackend()
		return fail()
	}
	return &indexing.Runtime{Catalog: sources, Jobs: jobs.New(sources.Pool(), sources, b, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()}), Backend: backend, Target: target, Space: space, Price: price, Reader: reader, OwnerSearch: ownerSearch, Log: log.With("component", "memory-indexing")}, closeBackend
}
