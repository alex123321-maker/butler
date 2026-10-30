package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/transport"
)

const frozenBrowserObservation = "browser.freeze_snapshot"

// Browser outcome observations never reconcile an effect or continue a model.
// The marker survives a crash even at the ordinary read-dispatch checkpoint.
type browserOutcomeCheckpoint struct {
	Version    int    `json:"version"`
	ToolCallID string `json:"tool_call_id"`
	EffectID   string `json:"effect_id"`
	ReadCallID string `json:"read_call_id"`
}

func (s *Service) observeUnknownBrowser(ctx context.Context, current *sessionv1.RunRecord, requested *transport.ToolCallRequest, original actionauth.PreparedAction, result *toolbrokerv1.ToolResult) error {
	if requested == nil || s.config.Checkpoints == nil || s.config.ActionAuthorizer == nil || s.transcript == nil {
		return nil
	}
	switch requested.ToolName {
	case "browser.click", "browser.fill", "browser.type":
	default:
		return nil
	}
	if original.EffectID == "" || original.Binding.Effect == actionauth.EffectRead || result.GetStatus() != "effect_unknown" || result.GetActionId() != original.EffectID || result.GetRunId() != current.GetRunId() || result.GetToolCallId() != requested.ToolCallRef || result.GetToolName() != requested.ToolName {
		return nil
	}
	var args struct {
		SessionID string `json:"browser_session_id"`
	}
	if json.Unmarshal([]byte(requested.ArgsJSON), &args) != nil {
		return nil
	}
	id, err := hex.DecodeString(args.SessionID)
	if err != nil || len(id) != 16 || hex.EncodeToString(id) != args.SessionID {
		return nil
	}
	executor, ok := s.config.Tools.(AuthorizedToolExecutor)
	if !ok {
		return nil
	}
	// Only the already recorded result may start a probe. Once its marker is
	// durable this function cannot schedule a second probe, including on error.
	stage, raw, err := s.config.Checkpoints.LoadCheckpoint(ctx, current.GetRunId())
	if err != nil {
		return err
	}
	var saved struct {
		Data struct {
			ToolCallID string          `json:"tool_call_id"`
			EffectID   string          `json:"effect_id"`
			Status     string          `json:"status"`
			Marker     json.RawMessage `json:"uncertain_browser"`
		} `json:"checkpoint_data"`
	}
	if stage != "tool_result_recorded" || json.Unmarshal([]byte(raw), &saved) != nil || len(saved.Data.Marker) != 0 || saved.Data.ToolCallID != requested.ToolCallRef || saved.Data.EffectID != original.EffectID || saved.Data.Status != "effect_unknown" {
		return nil
	}
	hash := sha256.Sum256([]byte(current.GetRunId() + "\x00" + requested.ToolCallRef))
	marker := browserOutcomeCheckpoint{Version: 1, ToolCallID: requested.ToolCallRef, EffectID: original.EffectID, ReadCallID: "browser-observe-" + hex.EncodeToString(hash[:16])}
	data := map[string]any{"tool_call_id": marker.ToolCallID, "effect_id": marker.EffectID, "uncertain_browser": marker}
	if err := s.checkpoint(ctx, current.GetRunId(), "browser_outcome_observing", data); err != nil {
		return err
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	call := &toolbrokerv1.ToolCall{RunId: current.GetRunId(), ToolCallId: marker.ReadCallID, ToolName: frozenBrowserObservation, ArgsJson: mustMarshalJSON(map[string]string{"browser_session_id": args.SessionID}), Status: "requested", AutonomyMode: current.GetAutonomyMode()}
	ctx, err = s.prepareActionSlot(ctx, call)
	if err != nil {
		return err
	}
	prepared, err := s.config.ActionAuthorizer.PrepareAction(ctx, call)
	if err != nil {
		return err
	}
	b := prepared.Binding
	argsHash, err := actionauth.ArgumentsHash(call.ArgsJson)
	if err != nil || prepared.RequiresApproval || prepared.EffectID == "" || prepared.EffectID == original.EffectID || b.ActionID == "" || b.ActionHash == "" || b.ContractHash == "" || b.ContractRevision != "1" || b.Effect != actionauth.EffectRead || b.RunID != call.RunId || b.ToolCallID != call.ToolCallId || b.ToolName != call.ToolName || b.ArgsHash != argsHash || b.Resource != "browser:"+args.SessionID+":" || len(b.CredentialRefs) != 0 || b.OwnerID != original.Binding.OwnerID || b.TaskID != original.Binding.TaskID || b.TaskRevision != original.Binding.TaskRevision || b.ConstraintsRevision != original.Binding.ConstraintsRevision {
		return fmt.Errorf("frozen observation permission unavailable")
	}
	mandateID := b.MandateID
	if mandateID == "" {
		mandateID = s.config.ActionMandateID
	}
	grant, err := s.config.ActionAuthorizer.AuthorizeAction(ctx, call, b.ActionID, "", mandateID)
	if err != nil || grant == "" {
		return fmt.Errorf("frozen observation authorization unavailable")
	}
	// Use the ordinary exact dispatch claim; the durable marker prevents this
	// read checkpoint from ever being mistaken for a model-resume checkpoint.
	dispatch := map[string]any{"tool_call_id": call.ToolCallId, "action_id": b.ActionID, "effect_id": prepared.EffectID, "authorization_id": grant, "uncertain_browser": marker}
	if err := s.checkpoint(ctx, call.RunId, "tool_dispatching", dispatch); err != nil {
		return err
	}
	started := time.Now().UTC()
	observed, err := executor.ExecuteAuthorizedToolCall(ctx, call, grant)
	finished := time.Now().UTC()
	if err != nil {
		return fmt.Errorf("frozen observation response unavailable")
	}
	if observed.GetRunId() != call.RunId || observed.GetToolCallId() != call.ToolCallId || observed.GetToolName() != call.ToolName || observed.GetActionId() != prepared.EffectID {
		return fmt.Errorf("frozen observation response identity mismatch")
	}
	// Stop the dispatch clock before bookkeeping. This context can only persist
	// the correlated response; it cannot call a runtime or extend live authority.
	cancel()
	ctx, persistCancel := context.WithTimeout(parentCtx, 3*time.Second)
	defer persistCancel()
	read := &transport.ToolCallRequest{ToolCallRef: call.ToolCallId, ToolName: call.ToolName, ArgsJSON: call.ArgsJson}
	if _, err := s.transcript.AppendToolCall(ctx, transcriptToolCallFromExecution(call.ToolCallId, call.RunId, read, call, observed, started, finished)); err != nil {
		return err
	}
	// Record only coarse metadata here. Sanitized page content stays in the
	// ordinary tool transcript and is not fed into an inference or final result.
	data["observation_status"] = "unavailable"
	var view struct {
		OK           bool   `json:"ok"`
		SessionID    string `json:"browser_session_id"`
		NetworkState string `json:"network_state"`
		Version      uint64 `json:"observation_version"`
	}
	if observed.GetStatus() == "completed" && observed.GetError() == nil && observed.GetEffectState() == string(actionauth.StateConfirmed) && json.Unmarshal([]byte(observed.GetResultJson()), &view) == nil && view.OK && view.SessionID == args.SessionID && view.NetworkState == "frozen" && view.Version != 0 {
		data["observation_status"] = "observed"
	}
	return s.checkpoint(ctx, call.RunId, "browser_outcome_observed", data)
}
