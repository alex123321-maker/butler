package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
)

func (s *MemoryCatalogServer) handleModelJobs(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v4/memory/model-") {
		return false
	}
	if s.ModelJobs == nil {
		writeJSON(w, 503, map[string]string{"error": "background memory unavailable"})
		return true
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/memory/model-jobs" {
		v, e := s.ModelJobs.ModelView(ctx)
		if e != nil {
			writeMemoryError(w, e)
		} else {
			writeJSON(w, 200, map[string]any{"policies": v, "worker_configured": s.ModelWorkerConfigured})
		}
		return true
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return true
	}
	var err error
	switch r.URL.Path {
	case "/api/v4/memory/model-policies":
		var q jobs.ModelPolicyRequest
		if !decodeMemory(w, r, &q) {
			return true
		}
		p, e := s.ModelJobs.CreateModelPolicy(ctx, q)
		if e != nil {
			writeMemoryError(w, e)
		} else {
			writeJSON(w, 201, p)
		}
		return true
	case "/api/v4/memory/model-policy-control":
		var q struct {
			ID    string `json:"policy_id"`
			State string `json:"state"`
		}
		if !decodeMemory(w, r, &q) {
			return true
		}
		err = s.ModelJobs.ControlModel(ctx, q.ID, q.State)
	case "/api/v4/memory/model-cancel":
		var q struct {
			ID string `json:"job_id"`
		}
		if !decodeMemory(w, r, &q) {
			return true
		}
		err = s.ModelJobs.CancelModelJob(ctx, q.ID)
	case "/api/v4/memory/model-worker-control":
		var q struct {
			State string `json:"state"`
		}
		if !decodeMemory(w, r, &q) {
			return true
		}
		if q.State == "enabled" && !s.ModelWorkerConfigured {
			writeJSON(w, 409, map[string]string{"error": "worker not configured"})
			return true
		}
		err = s.ModelJobs.SetModelControl(ctx, q.State)
	default:
		err = catalog.ErrInvalid
	}
	if err != nil {
		writeMemoryError(w, err)
	} else {
		writeJSON(w, 200, map[string]string{"status": "applied"})
	}
	return true
}

func decodeMemory(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
		writeMemoryError(w, catalog.ErrInvalid)
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	d.DisallowUnknownFields()
	if decodeOwnerRequest(d, v) != nil {
		writeMemoryError(w, catalog.ErrInvalid)
		return false
	}
	return true
}
