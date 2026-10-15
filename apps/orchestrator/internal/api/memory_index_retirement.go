package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/semantic"
)

func (s *MemoryCatalogServer) handleIndexRetirement(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/v4/memory/index-retire" {
		return false
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
		writeMemoryError(w, catalog.ErrInvalid)
		return true
	}
	var q semantic.RetireRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if decodeOwnerRequest(d, &q) != nil {
		writeMemoryError(w, catalog.ErrInvalid)
		return true
	}
	// This owner-only erasure path remains available with local models disabled.
	maintenance := &indexing.Runtime{Catalog: s.Store}
	out, err := maintenance.Retire(ctx, q)
	if err != nil {
		writeMemoryError(w, err)
		return true
	}
	writeJSON(w, http.StatusOK, out)
	return true
}
