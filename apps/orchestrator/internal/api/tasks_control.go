package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
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
type TaskControlServer struct{ tasks taskController }

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
			var req task.CreateRequest
			if err := decodeTaskBody(w, r, &req); err != nil {
				return
			}
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
		switch parts[1] {
		case "working":
			var req struct {
				ExpectedWorkingRevision int64             `json:"expected_working_revision"`
				Working                 task.WorkingState `json:"working"`
			}
			if decodeTaskBody(w, r, &req) != nil {
				return
			}
			record, err := s.tasks.SaveWorking(r.Context(), id, req.ExpectedWorkingRevision, req.Working)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"task": record})
		case "status":
			var req struct {
				ExpectedRevision int64  `json:"expected_revision"`
				Status           string `json:"status"`
				ResultID         string `json:"result_id"`
			}
			if decodeTaskBody(w, r, &req) != nil {
				return
			}
			record, err := s.tasks.Transition(r.Context(), id, req.ExpectedRevision, req.Status, req.ResultID)
			if err != nil {
				writeTaskError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"task": record})
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
