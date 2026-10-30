package browsercontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browserpage"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ContextReader interface {
	GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error)
}

// Store holds durable ownership/fences only, never screenshots or typed input.
// Runtime contexts and their finite lifetime remain transient capabilities.
type Store struct {
	Pool     *pgxpool.Pool
	Contexts ContextReader
	Runtime  Controller
}

// WaitRun is used by the execution kernel between provider turns, before
// proposals and before consuming provider events. Existing run cancellation,
// budget deadlines and session lease renewal remain authoritative.
func (s *Store) WaitRun(ctx context.Context, runID string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var owned bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM server_browser_sessions WHERE run_id=$1 AND mode='owner' AND expires_at>clock_timestamp())`, runID).Scan(&owned); err != nil {
			return ErrDenied
		}
		if !owned {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func hash(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func validID(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 32 && err == nil
}

// BrowserReadScope supplies broker-owned metadata for no-navigation context
// reads. A browser ID from another run, owner or Task is never a capability.
func (s *Store) BrowserReadScope(ctx context.Context, b actionauth.ActionBinding) (string, int64, error) {
	if s == nil || s.Pool == nil || !actionauth.BrowserContextRead(b) {
		return "", 0, ErrDenied
	}
	id := strings.TrimSuffix(strings.TrimPrefix(b.Resource, "browser:"), ":")
	if !validID(id) {
		return "", 0, ErrDenied
	}
	var resource string
	var epoch int64
	err := s.Pool.QueryRow(ctx, `SELECT observed_url,epoch FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4 AND mode='agent' AND expires_at>clock_timestamp()`, id, b.RunID, b.OwnerID, b.TaskID).Scan(&resource, &epoch)
	if err != nil || resource == "" {
		return "", 0, ErrDenied
	}
	return resource, epoch, nil
}

func (s *Store) checkReadRelease(ctx context.Context, b actionauth.ActionBinding, result *toolbrokerv1.ToolResult) error {
	if b.BrowserScopeURL == "" || b.ToolName == "browser.close" {
		return nil
	}
	var observed struct {
		URL string `json:"final_url"`
	}
	deny := func() error { result.ResultJson = "{}"; return ErrDenied }
	if !actionauth.BrowserContextRead(b) || json.Unmarshal([]byte(result.GetResultJson()), &observed) != nil || observed.URL == "" {
		return deny()
	}
	// The actual observed URL, rather than the previous URL or page text, must
	// satisfy current assignment and all goal deny scopes before any byte release.
	b.BrowserScopeURL = observed.URL
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return deny()
	}
	defer tx.Rollback(ctx)
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || actionauth.CheckTaskActionTx(ctx, tx, b, at) != nil {
		return deny()
	}
	id := strings.TrimSuffix(strings.TrimPrefix(b.Resource, "browser:"), ":")
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4 AND epoch=$5 AND mode='agent' AND expires_at>clock_timestamp() FOR SHARE`, id, b.RunID, b.OwnerID, b.TaskID, b.BrowserScopeEpoch).Scan(&current) != nil || !current {
		return deny()
	}
	if tx.Commit(ctx) != nil {
		return deny()
	}
	return nil
}

func (s *Store) Record(ctx context.Context, call *toolbrokerv1.ToolCall, grant actionauth.Grant, result *toolbrokerv1.ToolResult) error {
	if call == nil || !strings.HasPrefix(call.GetToolName(), "browser.") {
		return nil
	}
	if result == nil || result.GetStatus() != "completed" {
		return nil
	}
	if result.GetRunId() != call.GetRunId() || result.GetToolCallId() != call.GetToolCallId() || result.GetToolName() != call.GetToolName() {
		return ErrDenied
	}
	if browserpage.IsTool(call.ToolName) {
		return s.recordPageExecution(ctx, call, grant.Binding, result)
	}
	if err := s.checkReadRelease(ctx, grant.Binding, result); err != nil {
		return err
	}
	if call.GetToolName() == browserauth.Tool {
		if err := s.checkLoginRelease(ctx, call, grant.Binding, result); err != nil {
			return err
		}
	}
	if call.GetToolName() == "browser.open" {
		var output struct {
			URL     string    `json:"final_url"`
			ID      string    `json:"browser_session_id"`
			Expires time.Time `json:"expires_at"`
			OK      bool      `json:"ok"`
		}
		if json.Unmarshal([]byte(result.GetResultJson()), &output) != nil || !output.OK || !validID(output.ID) || !output.Expires.After(time.Now()) || output.Expires.After(time.Now().Add(16*time.Minute)) {
			return ErrDenied
		}
		_, err := s.Pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,expires_at,observed_url) VALUES($1,$2,$3,$4,$5,$6)`, output.ID, call.GetRunId(), grant.Binding.OwnerID, grant.Binding.TaskID, output.Expires, output.URL)
		return err
	}
	if call.GetToolName() == "browser.close" {
		var args struct {
			ID string `json:"browser_session_id"`
		}
		_ = json.Unmarshal([]byte(call.GetArgsJson()), &args)
		_, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET mode='closed',epoch=epoch+1,controller_hash='',updated_at=clock_timestamp() WHERE browser_session_id=$1 AND run_id=$2`, args.ID, call.GetRunId())
		return err
	}
	if call.GetToolName() == "browser.snapshot" || call.GetToolName() == "browser.freeze_snapshot" || call.GetToolName() == "browser.extract_text" {
		epoch, ok := admittedEpoch(ctx)
		if !ok {
			return ErrDenied
		}
		var args struct {
			ID string `json:"browser_session_id"`
		}
		_ = json.Unmarshal([]byte(call.GetArgsJson()), &args)
		var observed struct {
			URL string `json:"final_url"`
		}
		if json.Unmarshal([]byte(result.GetResultJson()), &observed) != nil || observed.URL == "" {
			return ErrDenied
		}
		_, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET fresh_needed=false,observed_url=$4 WHERE browser_session_id=$1 AND run_id=$2 AND mode='agent' AND epoch=$3`, args.ID, call.GetRunId(), epoch, observed.URL)
		return err
	}
	var observation struct {
		ID  string `json:"browser_session_id"`
		URL string `json:"final_url"`
		OK  bool   `json:"ok"`
	}
	if json.Unmarshal([]byte(result.GetResultJson()), &observation) == nil && observation.OK && observation.ID != "" && observation.URL != "" {
		_, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET observed_url=$3 WHERE browser_session_id=$1 AND run_id=$2 AND mode='agent'`, observation.ID, call.GetRunId(), observation.URL)
		return err
	}
	return nil
}

// Wait happens before Verify/Claim and budget spend. Expiry or cancellation
// cannot turn an unacknowledged owner release into permission to execute.
func (s *Store) Before(ctx context.Context, call *toolbrokerv1.ToolCall) (context.Context, error) {
	if !strings.HasPrefix(call.GetToolName(), "browser.") || call.GetToolName() == "browser.open" || call.GetToolName() == "browser.read" || call.GetToolName() == browserpage.Open {
		return ctx, nil
	}
	var args struct {
		ID string `json:"browser_session_id"`
	}
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil {
		return ctx, ErrDenied
	}
	timer := time.NewTicker(200 * time.Millisecond)
	defer timer.Stop()
	for {
		var mode string
		var fresh bool
		var epoch int64
		err := s.Pool.QueryRow(ctx, `SELECT mode,fresh_needed,epoch FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND expires_at>clock_timestamp()`, args.ID, call.GetRunId()).Scan(&mode, &fresh, &epoch)
		if err != nil {
			return ctx, ErrDenied
		}
		if mode == "agent" {
			if browserpage.IsTool(call.ToolName) {
				return s.beforePage(ctx, call, epoch)
			}
			var managed bool
			if s.Pool.QueryRow(ctx, `SELECT page_mode FROM server_browser_sessions WHERE browser_session_id=$1`, args.ID).Scan(&managed) != nil {
				return ctx, ErrDenied
			}
			if managed && call.ToolName != "browser.close" {
				return ctx, ErrDenied
			}
			if fresh && call.GetToolName() != "browser.snapshot" && call.GetToolName() != "browser.freeze_snapshot" && call.GetToolName() != "browser.extract_text" && call.GetToolName() != "browser.close" {
				return ctx, ErrDenied
			}
			return WithDispatchEpoch(ctx, epoch), nil
		}
		if mode != "owner" {
			return ctx, ErrDenied
		}
		select {
		case <-ctx.Done():
			return ctx, ErrDenied
		case <-timer.C:
		}
	}
}

func (s *Store) Control(ctx context.Context, command Command) (Reply, error) {
	var reply Reply
	if s == nil || s.Pool == nil || s.Contexts == nil || s.Runtime == nil || command.OwnerID == "" {
		return reply, ErrDenied
	}
	if command.Operation == "list" {
		if command.SessionID != "" || command.RunID != "" || command.Controller != "" {
			return reply, ErrDenied
		}
		rows, err := s.Pool.Query(ctx, `SELECT browser_session_id,run_id,task_id,expires_at,mode,epoch FROM server_browser_sessions WHERE owner_id=$1 AND expires_at>clock_timestamp() AND mode<>'closed' ORDER BY created_at DESC LIMIT 32`, command.OwnerID)
		if err != nil {
			return reply, ErrDenied
		}
		defer rows.Close()
		reply.Sessions = []Session{}
		for rows.Next() {
			var item Session
			if rows.Scan(&item.ID, &item.RunID, &item.TaskID, &item.Expires, &item.Mode, &item.Epoch) != nil {
				return Reply{}, ErrDenied
			}
			reply.Sessions = append(reply.Sessions, item)
		}
		reply.OK = rows.Err() == nil
		if !reply.OK {
			return Reply{}, ErrDenied
		}
		return reply, nil
	}
	if !validID(command.SessionID) {
		return reply, ErrDenied
	}
	var session Session
	var controllerHash string
	err := s.Pool.QueryRow(ctx, `SELECT browser_session_id,run_id,task_id,expires_at,mode,epoch,controller_hash FROM server_browser_sessions WHERE browser_session_id=$1 AND owner_id=$2 AND expires_at>clock_timestamp() AND mode<>'closed'`, command.SessionID, command.OwnerID).Scan(&session.ID, &session.RunID, &session.TaskID, &session.Expires, &session.Mode, &session.Epoch, &controllerHash)
	if err != nil {
		return reply, ErrDenied
	}
	// Releasing an existing controller only removes its write capability. It
	// must remain possible after task cancellation/lease expiry; it grants no
	// new viewing, input or agent authority. All other operations need live core
	// authority. Owner mapping and controller/epoch checks still apply below.
	if command.Operation != "release" {
		a, err := s.Contexts.GetActionContext(ctx, session.RunID)
		if err != nil || a.OwnerID != command.OwnerID || a.RunID != session.RunID || a.TaskID != session.TaskID || a.LeaseID == "" || !a.LeaseExpiresAt.After(time.Now()) || a.TaskID != "" && (a.TaskStatus != "active" || a.TaskRevision != a.PinnedTaskRevision) {
			return reply, ErrDenied
		}
		switch strings.TrimPrefix(a.RunState, "RUN_STATE_") {
		case "PREPARING", "MODEL_RUNNING", "AWAITING_MODEL_RESUME", "TOOL_PENDING", "TOOL_RUNNING", "AWAITING_APPROVAL", "preparing", "model_running", "awaiting_model_resume", "tool_pending", "tool_running", "awaiting_approval":
		default:
			return reply, ErrDenied
		}
	}
	// This is a direct authenticated human action, not delegated agent authority.
	// It grants no action mandate and does not reconcile any unknown agent effect.
	command.RunID = session.RunID
	if command.Operation == "take" {
		if command.Epoch != session.Epoch || !validID(command.Controller) {
			return reply, ErrDenied
		}
		row, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET mode='owner',fresh_needed=true,epoch=epoch+1,controller_hash=$4,updated_at=clock_timestamp() WHERE browser_session_id=$1 AND owner_id=$2 AND epoch=$3 AND expires_at>clock_timestamp()`, session.ID, command.OwnerID, session.Epoch, hash(command.Controller))
		if err != nil || row.RowsAffected() != 1 {
			return reply, ErrDenied
		}
		command.Epoch++
	} else if command.Operation != "frame" {
		if session.Mode != "owner" || command.Epoch != session.Epoch || !validID(command.Controller) || hash(command.Controller) != controllerHash {
			return reply, ErrDenied
		}
	} else {
		// Viewers have no write capability, even if they know an epoch.
		command.Controller = ""
		command.Epoch = session.Epoch
	}
	ownerID := command.OwnerID
	command.OwnerID = ""
	var eventID int64
	if command.Operation != "frame" {
		if err = s.Pool.QueryRow(ctx, `INSERT INTO server_browser_control_events(browser_session_id,owner_id,epoch,operation,status) VALUES($1,$2,$3,$4,'started') RETURNING event_id`, session.ID, ownerID, command.Epoch, command.Operation).Scan(&eventID); err != nil {
			return Reply{}, ErrDenied
		}
	}
	reply, err = s.Runtime.Control(ctx, command)
	if eventID != 0 {
		status := "acknowledged"
		if err != nil || !reply.OK {
			status = "unknown"
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		_, finishErr := s.Pool.Exec(finishCtx, `UPDATE server_browser_control_events SET status=$2,finished_at=clock_timestamp() WHERE event_id=$1`, eventID, status)
		cancel()
		if finishErr != nil {
			return Reply{}, ErrDenied
		}
	}
	if err != nil || !reply.OK {
		return Reply{}, ErrDenied
	}
	if command.Operation == "release" {
		row, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET mode='agent',epoch=epoch+1,controller_hash='',updated_at=clock_timestamp() WHERE browser_session_id=$1 AND mode='owner' AND epoch=$2 AND controller_hash=$3`, session.ID, command.Epoch, hash(command.Controller))
		if err != nil || row.RowsAffected() != 1 {
			return Reply{}, ErrDenied
		}
	}
	return reply, nil
}
