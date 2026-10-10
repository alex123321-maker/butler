package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
)

type taskController interface {
	Create(context.Context, task.CreateRequest) (task.Record, error)
	Get(context.Context, string) (task.Record, error)
	List(context.Context, string, int) ([]task.Record, error)
	SaveWorking(context.Context, string, int64, task.WorkingState) (task.Record, error)
	Transition(context.Context, string, int64, string, string) (task.Record, error)
	SaveVerifiedResult(context.Context, task.Result) (task.Result, error)
	Results(context.Context, string) ([]task.Result, error)
	RunIDs(context.Context, string) ([]string, error)
}

// TaskControlServer is a trusted owner control surface. It is deliberately absent
// from the model tool catalog; model final events cannot call completion.
type TaskControlServer struct {
	tasks     taskController
	controls  *task.Controller
	ownerAuth *OwnerControlAuth
}

func (s *TaskControlServer) SetOwnerControlAuth(auth *OwnerControlAuth) { s.ownerAuth = auth }
func (s *TaskControlServer) SetControls(c *task.Controller)             { s.controls = c }
func (s *TaskControlServer) authenticate(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	actor, ok := s.ownerAuth.Authenticate(r)
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "authenticated task owner required"})
		return r, false
	}
	return r.WithContext(owneridentity.WithActor(r.Context(), actor)), true
}

func NewTaskControlServer(tasks taskController) *TaskControlServer {
	return &TaskControlServer{tasks: tasks}
}

func (s *TaskControlServer) HandleCollection() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := s.tasks.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("session_key")), 50)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"tasks": items})
		case http.MethodPost:
			var authenticated bool
			r, authenticated = s.authenticate(w, r)
			if !authenticated {
				return
			}
			var req task.CreateRequest
			if err := decodeTaskBody(w, r, &req); err != nil {
				return
			}
			actor, _ := owneridentity.FromContext(r.Context())
			if req.OwnerID != "" && req.OwnerID != actor.OwnerID {
				writeTaskError(w, task.ErrUnauthorized)
				return
			}
			req.OwnerID = actor.OwnerID
			record, err := s.tasks.Create(r.Context(), req)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 201, map[string]any{"task": record})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (s *TaskControlServer) HandleItem(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
			w.WriteHeader(404)
			return
		}
		id := parts[0]
		if len(parts) == 2 && parts[1] == "controls" && r.Method == http.MethodGet && s.controls != nil {
			history, err := s.controls.History(r.Context(), id)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"controls": history})
			return
		}
		if len(parts) == 1 && r.Method == http.MethodGet {
			record, err := s.tasks.Get(r.Context(), id)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			results, err := s.tasks.Results(r.Context(), id)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			runs, err := s.tasks.RunIDs(r.Context(), id)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"task": record, "run_ids": runs, "results": results})
			return
		}
		if len(parts) != 2 || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var authenticated bool
		r, authenticated = s.authenticate(w, r)
		if !authenticated {
			return
		}
		if s.tasks != nil {
			record, err := s.tasks.Get(r.Context(), id)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			actor, _ := owneridentity.FromContext(r.Context())
			if record.OwnerID != actor.OwnerID {
				writeTaskError(w, task.ErrUnauthorized)
				return
			}
		}
		switch parts[1] {
		case "control":
			if s.controls == nil {
				writeTaskError(w, task.ErrConflict)
				return
			}
			if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
				writeTaskError(w, task.ErrInvalid)
				return
			}
			var req task.ControlRequest
			if decodeTaskBody(w, r, &req) != nil {
				return
			}
			if req.TaskID != "" && req.TaskID != id {
				writeTaskError(w, task.ErrInvalid)
				return
			}
			req.TaskID = id
			if req.ExpectedRevision <= 0 {
				writeTaskError(w, task.ErrInvalid)
				return
			}
			result, err := s.controls.Apply(r.Context(), req)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, result)
		case "working":
			writeJSON(w, 409, map[string]string{"error": "working state is execution-owned; use a versioned owner amendment"})
			return
		case "status":
			var req struct {
				ExpectedRevision int64  `json:"expected_revision"`
				Status           string `json:"status"`
				ResultID         string `json:"result_id"`
			}
			if decodeTaskBody(w, r, &req) != nil {
				return
			}
			if s.controls == nil || req.ExpectedRevision <= 0 || strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
				writeTaskError(w, task.ErrInvalid)
				return
			}
			kind := task.ControlStatus
			switch req.Status {
			case "cancelled":
				kind = task.ControlStop
			case "paused":
				kind = task.ControlPause
			case "active":
				kind = task.ControlResume
			}
			result, err := s.controls.Apply(r.Context(), task.ControlRequest{TaskID: id, ExpectedRevision: req.ExpectedRevision, Kind: kind, Status: req.Status, ResultID: req.ResultID})
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, result)
		case "results":
			var req struct {
				TaskRevision     int64    `json:"task_revision"`
				RunID            string   `json:"run_id"`
				Summary          string   `json:"summary"`
				EvidenceRefs     []string `json:"evidence_refs"`
				VerifiedCriteria []string `json:"verified_criteria"`
				OwnerConfirmed   bool     `json:"owner_confirmed"`
			}
			if decodeTaskBody(w, r, &req) != nil {
				return
			}
			if !req.OwnerConfirmed {
				writeTaskError(w, task.ErrInvalid)
				return
			}
			result, err := s.tasks.SaveVerifiedResult(r.Context(), task.Result{TaskID: id, TaskRevision: req.TaskRevision, RunID: req.RunID, Summary: req.Summary, EvidenceRefs: req.EvidenceRefs, VerifiedCriteria: req.VerifiedCriteria, Verifier: "owner"})
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 201, map[string]any{"result": result})
		default:
			w.WriteHeader(404)
		}
	})
}

func decodeTaskBody(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(value)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid task request body"})
	}
	return err
}

func writeTaskError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "task operation failed"
	switch {
	case errors.Is(err, task.ErrUnauthorized):
		status = 403
		message = err.Error()
	case errors.Is(err, task.ErrNotFound):
		status = 404
		message = err.Error()
	case errors.Is(err, task.ErrConflict):
		status = 409
		message = err.Error()
	case errors.Is(err, task.ErrInvalid):
		status = 400
		message = err.Error()
	}
	writeJSON(w, status, map[string]string{"error": message})
}
