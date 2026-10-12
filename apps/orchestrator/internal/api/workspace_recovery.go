package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/workspaceexec"
)

type workspaceRecoveryController interface {
	Get(context.Context, string) (workspaceexec.RecoveryStatus, error)
	ListForTask(context.Context, string) ([]workspaceexec.RecoveryStatus, error)
	RequestDiscard(context.Context, workspaceexec.DiscardRequest) (workspaceexec.RecoveryStatus, error)
}
type WorkspaceRecoveryServer struct {
	controller workspaceRecoveryController
	auth       *OwnerControlAuth
}

func NewWorkspaceRecoveryServer(controller workspaceRecoveryController, auth *OwnerControlAuth) *WorkspaceRecoveryServer {
	return &WorkspaceRecoveryServer{controller: controller, auth: auth}
}

var workspaceRecoveryID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,255}$`)

func (s *WorkspaceRecoveryServer) authenticated(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := s.auth.Authenticate(r)
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "authenticated task owner required"})
		return r, false
	}
	if s.controller == nil {
		writeJSON(w, 503, map[string]string{"error": "workspace recovery unavailable"})
		return r, false
	}
	return r.WithContext(owneridentity.WithActor(r.Context(), actor)), true
}
func (s *WorkspaceRecoveryServer) Handle(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if len(parts) != 2 || !workspaceRecoveryID.MatchString(parts[0]) || parts[1] != "recovery" && parts[1] != "discard" {
			w.WriteHeader(404)
			return
		}
		if parts[1] == "recovery" && r.Method != http.MethodGet || parts[1] == "discard" && r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var ok bool
		r, ok = s.authenticated(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var status workspaceexec.RecoveryStatus
		var err error
		if parts[1] == "recovery" {
			status, err = s.controller.Get(ctx, parts[0])
		} else {
			delivery := r.Header.Get("X-Butler-Delivery-ID")
			if delivery == "" || strings.TrimSpace(delivery) != delivery || len(delivery) > 512 {
				writeJSON(w, 400, map[string]string{"error": "stable owner delivery ID required"})
				return
			}
			req, decodeErr := decodeWorkspaceDiscard(w, r)
			if decodeErr != nil || req.WorkspaceID != parts[0] {
				writeJSON(w, 400, map[string]string{"error": "exact current workspace discard proof required"})
				return
			}
			status, err = s.controller.RequestDiscard(ctx, req)
		}
		if err != nil {
			writeWorkspaceRecoveryError(w, err)
			return
		}
		writeJSON(w, 200, status)
	})
}
func (s *WorkspaceRecoveryServer) HandleTaskList(id string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		if !workspaceRecoveryID.MatchString(id) {
			w.WriteHeader(400)
			return
		}
		var ok bool
		r, ok = s.authenticated(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		statuses, err := s.controller.ListForTask(ctx, id)
		if err != nil {
			writeWorkspaceRecoveryError(w, err)
			return
		}
		if statuses == nil {
			statuses = []workspaceexec.RecoveryStatus{}
		}
		writeJSON(w, 200, map[string]any{"workspaces": statuses, "limit": 50, "may_have_more": len(statuses) == 50})
	})
}
func writeWorkspaceRecoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workspaceexec.ErrDenied):
		writeJSON(w, 403, map[string]string{"error": "workspace recovery denied"})
	case errors.Is(err, workspaceexec.ErrRecoveryConflict):
		writeJSON(w, 409, map[string]string{"error": "workspace recovery proof changed or is unavailable"})
	default:
		writeJSON(w, 503, map[string]string{"error": "workspace recovery unavailable"})
	}
}

// Exact finite keys reject aliases, duplicates, null CAS values and trailing
// objects. No host path, executor proof, secret, caller receipt or owner is input.
func decodeWorkspaceDiscard(w http.ResponseWriter, r *http.Request) (workspaceexec.DiscardRequest, error) {
	var req workspaceexec.DiscardRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return req, errors.New("object required")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return req, err
		}
		key, ok := token.(string)
		if !ok {
			return req, errors.New("key required")
		}
		switch key {
		case "workspace_id", "resource_version", "workspace_generation", "task_revision", "constraints_revision", "previous_recovery_id":
		default:
			return req, errors.New("unknown key")
		}
		if _, exists := fields[key]; exists {
			return req, errors.New("duplicate key")
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil || string(value) == "null" {
			return req, errors.New("non-null finite value required")
		}
		fields[key] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return req, errors.New("object required")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return req, errors.New("trailing value")
	}
	for _, key := range []string{"workspace_id", "resource_version", "workspace_generation", "task_revision", "constraints_revision"} {
		if _, exists := fields[key]; !exists {
			return req, errors.New("missing CAS")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return req, err
	}
	if err = json.Unmarshal(raw, &req); err != nil {
		return req, err
	}
	if !workspaceRecoveryID.MatchString(req.WorkspaceID) || req.ResourceVersion <= 0 || req.TaskRevision <= 0 || req.ConstraintsRevision <= 0 || req.WorkspaceGeneration < 0 || req.PreviousRecoveryID != "" && !workspaceRecoveryID.MatchString(req.PreviousRecoveryID) {
		return req, errors.New("invalid CAS")
	}
	return req, nil
}
