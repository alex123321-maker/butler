package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/transport"
)

// handleToolBatch processes a batch of tool calls sequentially.
func (s *Service) handleToolBatch(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, batch *transport.ToolCallBatch) (string, *sessionv1.RunRecord, error) {
	if batch == nil || len(batch.ToolCalls) == 0 {
		return "", current, nil
	}
	var finalMessage string
	for _, toolCall := range batch.ToolCalls {
		resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, &toolCall)
		if err != nil {
			return "", updated, err
		}
		current = updated
		resumedFinal, updated, err := s.consumeModelStream(ctx, runLog, current, sessionKey, leaseID, resumed)
		if err != nil {
			return "", updated, err
		}
		current = updated
		if strings.TrimSpace(resumedFinal) != "" {
			finalMessage = resumedFinal
		}
	}
	return finalMessage, current, nil
}

// handleToolCall executes a single tool call, including approval flow if configured.
func (s *Service) handleToolCall(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, leaseID string, requested *transport.ToolCallRequest) (transport.EventStream, *sessionv1.RunRecord, error) {
	return s.handleActionToolCall(ctx, runLog, current, leaseID, requested, "")
}

func (s *Service) resumeApprovedToolCall(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, leaseID string, requested *transport.ToolCallRequest, approvalID string) (transport.EventStream, *sessionv1.RunRecord, error) {
	if approvalID == "" {
		return nil, current, fmt.Errorf("durable action approval is required to resume")
	}
	return s.handleActionToolCall(ctx, runLog, current, leaseID, requested, approvalID)
}

func (s *Service) handleActionToolCall(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, leaseID string, requested *transport.ToolCallRequest, approvalID string) (transport.EventStream, *sessionv1.RunRecord, error) {
	if requested == nil {
		return nil, current, fmt.Errorf("tool call request is required")
	}
	if s.config.Tools == nil {
		return nil, current, fmt.Errorf("tool executor is not configured")
	}
	next := current
	var err error
	if approvalID == "" {
		next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_TOOL_PENDING, leaseID, "", "")
		if err != nil {
			return nil, current, fmt.Errorf("mark run tool_pending: %w", err)
		}
	} else if current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
		return nil, current, fmt.Errorf("approved resume requires a durably suspended run")
	}
	current = next

	toolCallID := requested.ToolCallRef
	if strings.TrimSpace(toolCallID) == "" {
		toolCallID = fallbackToolCallID(current.GetRunId(), time.Now().UTC())
		requested.ToolCallRef = toolCallID
	}
	brokerCall := &toolbrokerv1.ToolCall{ToolCallId: toolCallID, RunId: current.GetRunId(), ToolName: requested.ToolName, ArgsJson: requested.ArgsJSON, Status: "requested", AutonomyMode: current.GetAutonomyMode()}

	if refs := extractCredentialRefs(requested.ArgsJSON); len(refs) > 0 {
		brokerCall.CredentialRefs = refs
	}

	// A missing or unavailable policy service cannot authorize execution.
	if s.config.ActionAuthorizer == nil {
		return nil, current, fmt.Errorf("tool authorization unavailable: exact action authorizer is not configured")
	}
	prepared, checkErr := s.config.ActionAuthorizer.PrepareAction(ctx, brokerCall)
	if checkErr != nil {
		return nil, current, fmt.Errorf("prepare exact action authorization: %w", checkErr)
	}
	if prepared.Binding.RunID != brokerCall.GetRunId() || prepared.Binding.ToolCallID != toolCallID || prepared.Binding.ToolName != brokerCall.GetToolName() || prepared.Binding.ActionID == "" || prepared.Binding.ActionHash == "" {
		return nil, current, fmt.Errorf("action authorizer returned an incompatible binding")
	}
	{
		if prepared.RequiresApproval && approvalID == "" {
			if s.config.Checkpoints != nil {
				suspended, suspendErr := s.suspendApproval(ctx, current, requested, prepared)
				return nil, suspended, suspendErr
			}
			if s.config.ApprovalGate == nil {
				return nil, current, fmt.Errorf("tool authorization unavailable: approval gate is not configured")
			}
			// Register before durable publication and notification, so an immediate
			// user response cannot arrive before the waiter exists.
			pendingApproval, registerErr := s.config.ApprovalGate.Register(ctx, toolCallID)
			if registerErr != nil {
				return nil, current, fmt.Errorf("register approval: %w", registerErr)
			}
			defer pendingApproval.Close()
			var approvalRecord approvals.Record
			if s.config.ApprovalService != nil {
				createdApproval, createErr := s.config.ApprovalService.CreatePendingApproval(ctx, approvals.CreatePendingParams{
					RunID:        current.GetRunId(),
					SessionKey:   current.GetSessionKey(),
					ToolCallID:   toolCallID,
					ApprovalType: approvals.ApprovalTypeToolCall,
					RequestedVia: approvalRequestedViaForSession(current.GetSessionKey()),
					ToolName:     requested.ToolName,
					ArgsJSON:     requested.ArgsJSON,
					PayloadJSON:  `{"kind":"tool_call"}`,
					RiskLevel:    deriveApprovalRiskLevel(requested.ToolName),
					Summary:      fmt.Sprintf("Approval required for tool %s", requested.ToolName),
					RequestedAt:  time.Now().UTC(),
				})
				if createErr != nil {
					return nil, current, fmt.Errorf("create pending approval: %w", createErr)
				}
				approvalRecord = createdApproval
			}

			next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, leaseID, "", "")
			if err != nil {
				return nil, current, fmt.Errorf("mark run awaiting_approval: %w", err)
			}
			current = next

			if deliveryErr := s.config.Delivery.DeliverApprovalRequest(ctx, ApprovalRequest{
				RunID:        current.GetRunId(),
				SessionKey:   current.GetSessionKey(),
				ApprovalID:   approvalRecord.ApprovalID,
				ApprovalType: approvalRecord.ApprovalType,
				ToolCallID:   toolCallID,
				ToolName:     requested.ToolName,
				ArgsJSON:     requested.ArgsJSON,
				PayloadJSON:  approvalRecord.PayloadJSON,
			}); deliveryErr != nil {
				return nil, current, fmt.Errorf("deliver approval request: %w", deliveryErr)
			}

			s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventApprovalRequested, map[string]any{
				"tool_call_id": toolCallID,
				"tool_name":    requested.ToolName,
				"args_preview": truncateForObservability(requested.ArgsJSON, 200),
			})

			resp, waitErr := pendingApproval.Wait()
			if waitErr != nil {
				if s.config.ApprovalService != nil {
					_, _, _ = s.config.ApprovalService.ResolveByToolCall(ctx, approvals.ResolveByToolCallParams{
						ToolCallID:       toolCallID,
						Approved:         false,
						ResolvedVia:      approvals.ResolvedViaSystem,
						ResolvedBy:       "orchestrator",
						ResolutionReason: "approval wait cancelled or timed out",
						ResolvedAt:       time.Now().UTC(),
						ActorType:        "system",
						ActorID:          "orchestrator",
					})
				}
				return nil, current, fmt.Errorf("wait for approval: %w", waitErr)
			}
			if s.config.ApprovalService != nil {
				resolvedVia := approvals.ResolvedViaSystem
				actorType := "system"
				actorID := "orchestrator"
				if strings.EqualFold(resp.Channel, "telegram") {
					resolvedVia = approvals.ResolvedViaTelegram
					actorType = "telegram"
					if strings.TrimSpace(resp.ResolvedBy) != "" {
						actorID = resp.ResolvedBy
					}
				}
				resolvedApproval, _, resolveErr := s.config.ApprovalService.ResolveByToolCall(ctx, approvals.ResolveByToolCallParams{
					ToolCallID:       toolCallID,
					Approved:         resp.Approved,
					ResolvedVia:      resolvedVia,
					ResolvedBy:       actorID,
					ResolutionReason: approvalResolutionReason(resp.Approved),
					ResolvedAt:       time.Now().UTC(),
					ActorType:        actorType,
					ActorID:          actorID,
				})
				if resolveErr != nil {
					return nil, current, fmt.Errorf("resolve durable approval: %w", resolveErr)
				}
				// PostgreSQL is authoritative if another channel already decided.
				resp.Approved = resolvedApproval.Status == approvals.StatusApproved
				approvalID = resolvedApproval.ApprovalID
			}
			if !resp.Approved {
				s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventApprovalResolved, map[string]any{
					"tool_call_id": toolCallID,
					"tool_name":    requested.ToolName,
					"approved":     false,
				})
				rejectedResult := &toolbrokerv1.ToolResult{
					ToolCallId: toolCallID,
					RunId:      current.GetRunId(),
					ToolName:   requested.ToolName,
					Status:     "rejected",
					ResultJson: `{"rejected":true,"reason":"user rejected tool call"}`,
				}

				rejectedAt := time.Now().UTC()
				if _, err := s.transcript.AppendToolCall(ctx, transcriptToolCallFromExecution(toolCallID, current.GetRunId(), requested, brokerCall, rejectedResult, rejectedAt, rejectedAt)); err != nil {
					runLog.Warn("failed to append rejected tool call to transcript", slog.String("error", err.Error()))
				}

				next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, leaseID, "", "")
				if err != nil {
					return nil, current, fmt.Errorf("mark run awaiting_model_resume after rejection: %w", err)
				}
				current = next
				stream, submitErr := s.provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: current.GetRunId(), ProviderSessionRef: providerSessionRefFromRun(current), ToolCallRef: toolCallID, ToolResultJSON: toolResultEnvelope(rejectedResult)})
				if submitErr != nil {
					return nil, current, fmt.Errorf("submit rejected tool result: %w", submitErr)
				}
				next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, leaseID, "", "")
				if err != nil {
					return nil, current, fmt.Errorf("mark run model_running after rejection: %w", err)
				}
				current = next
				return stream, current, nil
			}
			runLog.Info("tool call approved", slog.String("tool_call_id", toolCallID), slog.String("tool_name", requested.ToolName))
			s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventApprovalResolved, map[string]any{
				"tool_call_id": toolCallID,
				"tool_name":    requested.ToolName,
				"approved":     true,
			})
		}
	}
	grantID, authorizeErr := s.config.ActionAuthorizer.AuthorizeAction(ctx, brokerCall, prepared.Binding.ActionID, approvalID, s.config.ActionMandateID)
	if authorizeErr != nil {
		return nil, current, fmt.Errorf("authorize exact action dispatch: %w", authorizeErr)
	}
	executor, ok := s.config.Tools.(AuthorizedToolExecutor)
	if !ok || grantID == "" {
		return nil, current, fmt.Errorf("authorized tool executor is not configured")
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_TOOL_RUNNING, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run tool_running: %w", err)
	}
	current = next

	startedAt := time.Now().UTC()
	s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventToolStarted, map[string]any{
		"tool_call_id": toolCallID,
		"tool_name":    requested.ToolName,
		"args_preview": truncateForObservability(requested.ArgsJSON, 200),
	})

	// Notify channel that a tool call is starting.
	if deliveryErr := s.config.Delivery.DeliverToolCallEvent(ctx, ToolCallEvent{
		RunID:      current.GetRunId(),
		SessionKey: current.GetSessionKey(),
		ToolCallID: toolCallID,
		ToolName:   requested.ToolName,
		ArgsJSON:   requested.ArgsJSON,
		Status:     "started",
	}); deliveryErr != nil {
		runLog.Warn("failed to deliver tool call started event", slog.String("tool_name", requested.ToolName), slog.String("error", deliveryErr.Error()))
	}

	workingStatus := "running"
	if updateErr := s.updateWorkingMemoryCheckpoint(ctx, current.GetSessionKey(), current.GetRunId(), workingStatus, requested.ToolName, requested.ArgsJSON); updateErr != nil {
		runLog.Warn("working memory update failed before tool execution", slog.String("error", updateErr.Error()))
	}
	if updateErr := s.saveTransientWorkingState(ctx, current.GetRunId(), current.GetSessionKey(), "tool_running", map[string]any{"tool_name": requested.ToolName, "args_json": normalizeJSON(requested.ArgsJSON, "{}")}); updateErr != nil {
		runLog.Warn("transient working memory update failed before tool execution", slog.String("error", updateErr.Error()))
	}
	if err := s.checkpoint(ctx, current.GetRunId(), "tool_dispatching", map[string]any{"tool_call_id": toolCallID, "action_id": prepared.Binding.ActionID, "authorization_id": grantID}); err != nil {
		return nil, current, fmt.Errorf("persist tool dispatch checkpoint: %w", err)
	}
	result, err := executor.ExecuteAuthorizedToolCall(ctx, brokerCall, grantID)
	if err != nil {
		return nil, current, fmt.Errorf("execute tool call %s: %w", requested.ToolName, err)
	}
	finishedAt := time.Now().UTC()
	if updateErr := s.updateWorkingMemoryCheckpoint(ctx, current.GetSessionKey(), current.GetRunId(), "active", requested.ToolName, toolResultPayload(result)); updateErr != nil {
		runLog.Warn("working memory update failed after tool execution", slog.String("error", updateErr.Error()))
	}
	if updateErr := s.saveTransientWorkingState(ctx, current.GetRunId(), current.GetSessionKey(), "awaiting_model_resume", map[string]any{"tool_name": requested.ToolName, "result_json": toolResultPayload(result)}); updateErr != nil {
		runLog.Warn("transient working memory update failed after tool execution", slog.String("error", updateErr.Error()))
	}
	if _, err := s.transcript.AppendToolCall(ctx, transcriptToolCallFromExecution(toolCallID, current.GetRunId(), requested, brokerCall, result, startedAt, finishedAt)); err != nil {
		return nil, current, fmt.Errorf("append tool transcript: %w", err)
	}
	if err := s.checkpoint(ctx, current.GetRunId(), "tool_result_recorded", map[string]any{"tool_call_id": toolCallID, "action_id": prepared.Binding.ActionID}); err != nil {
		return nil, current, fmt.Errorf("persist known tool result checkpoint: %w", err)
	}
	if result.GetStatus() == "effect_unknown" {
		return nil, current, ErrActionOutcomeUnknown
	}
	if s.config.Artifacts != nil {
		if _, saveErr := s.config.Artifacts.SaveToolResult(ctx, current.GetRunId(), current.GetSessionKey(), toolCallID, requested.ToolName, normalizeToolStatus(result.GetStatus()), toolResultPayload(result), finishedAt); saveErr != nil {
			runLog.Warn("failed to persist tool_result artifact", slog.String("tool_call_id", toolCallID), slog.String("error", saveErr.Error()))
		}
	}

	s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventToolCompleted, map[string]any{
		"tool_call_id":   toolCallID,
		"tool_name":      requested.ToolName,
		"status":         normalizeToolStatus(result.GetStatus()),
		"duration_ms":    finishedAt.Sub(startedAt).Milliseconds(),
		"result_preview": truncateForObservability(result.GetResultJson(), 200),
		"has_error":      result.GetError() != nil,
	})

	// Notify channel that the tool call finished.
	toolStatus := "completed"
	if result.GetError() != nil {
		toolStatus = "failed"
	}
	if deliveryErr := s.config.Delivery.DeliverToolCallEvent(ctx, ToolCallEvent{
		RunID:      current.GetRunId(),
		SessionKey: current.GetSessionKey(),
		ToolCallID: toolCallID,
		ToolName:   requested.ToolName,
		Status:     toolStatus,
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
	}); deliveryErr != nil {
		runLog.Warn("failed to deliver tool call completed event", slog.String("tool_name", requested.ToolName), slog.String("error", deliveryErr.Error()))
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run awaiting_model_resume: %w", err)
	}
	current = next

	stream, err := s.resumeProviderWithToolResult(ctx, current, toolCallID, toolResultEnvelope(result))
	if err != nil {
		return nil, current, fmt.Errorf("submit tool result: %w", err)
	}
	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run model_running after tool: %w", err)
	}
	current = next
	return stream, current, nil
}

func deriveApprovalRiskLevel(toolName string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if strings.Contains(name, "delete") || strings.Contains(name, "write") || strings.Contains(name, "exec") {
		return "high"
	}
	if strings.Contains(name, "browser") || strings.Contains(name, "http") {
		return "medium"
	}
	return "low"
}

func approvalResolutionReason(approved bool) string {
	if approved {
		return "approved via telegram callback"
	}
	return "rejected via telegram callback"
}

func approvalRequestedViaForSession(sessionKey string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sessionKey)), "telegram:") {
		return approvals.RequestedViaTelegram
	}
	return approvals.RequestedViaWeb
}

// toolDefinitionsFromContracts converts tool contracts to transport-layer tool definitions.
func toolDefinitionsFromContracts(contracts []*toolbrokerv1.ToolContract) []transport.ToolDefinition {
	if len(contracts) == 0 {
		return nil
	}
	defs := make([]transport.ToolDefinition, 0, len(contracts))
	for _, contract := range contracts {
		if contract == nil || strings.EqualFold(contract.GetStatus(), "disabled") {
			continue
		}
		defs = append(defs, transport.ToolDefinition{
			Name:        contract.GetToolName(),
			Description: contract.GetDescription(),
			SchemaJSON:  contract.GetInputSchemaJson(),
		})
	}
	return defs
}

// toolSummaryFromContracts builds a human-readable tool summary for prompt injection.
func toolSummaryFromContracts(contracts []*toolbrokerv1.ToolContract) string {
	if len(contracts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		if contract == nil || strings.EqualFold(contract.GetStatus(), "disabled") {
			continue
		}
		parts := []string{contract.GetToolName()}
		if class := strings.TrimSpace(contract.GetToolClass()); class != "" {
			parts[0] += " [" + class + "]"
		}
		details := []string{}
		if summary := compactSentence(contract.GetDescription()); summary != "" {
			details = append(details, summary)
		}
		if contract.GetSupportsCredentialRefs() {
			details = append(details, "credential refs")
		}
		if contract.GetRequiresApproval() {
			details = append(details, "approval")
		}
		line := "- " + parts[0]
		if len(details) > 0 {
			line += ": " + strings.Join(details, "; ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func extractCredentialRefs(argsJSON string) []*toolbrokerv1.CredentialRef {
	if strings.TrimSpace(argsJSON) == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(argsJSON), &parsed); err != nil {
		return nil
	}

	var refs []*toolbrokerv1.CredentialRef
	var walk func(v any)
	walk = func(v any) {
		switch val := v.(type) {
		case map[string]any:
			if t, ok := val["type"].(string); ok && t == "credential_ref" {
				alias, _ := val["alias"].(string)
				field, _ := val["field"].(string)
				if alias != "" && field != "" {
					refs = append(refs, &toolbrokerv1.CredentialRef{
						Type:  t,
						Alias: alias,
						Field: field,
					})
				}
			}
			for _, child := range val {
				walk(child)
			}
		case []any:
			for _, child := range val {
				walk(child)
			}
		}
	}
	walk(parsed)
	return refs
}
