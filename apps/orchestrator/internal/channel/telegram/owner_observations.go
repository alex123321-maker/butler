package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
)

type OwnerTaskReader interface {
	OwnerOverview(context.Context, string) (task.OwnerOverview, error)
}
type OwnerAuthority interface {
	RevokeMandate(context.Context, goal.MandateRevokeRequest) (goal.Receipt, error)
}

func (a *Adapter) SetOwnerTaskReader(reader OwnerTaskReader)  { a.ownerTasks = reader }
func (a *Adapter) SetOwnerAuthority(authority OwnerAuthority) { a.ownerAuthority = authority }

func (a *Adapter) handleOwnerObservationCommand(ctx context.Context, update Update) (bool, error) {
	message := update.Message
	if message == nil {
		return false, nil
	}
	command, ok := telegramCommand(message.Text)
	if !ok || command != "task" && command != "mandate_revoke" {
		return false, nil
	}
	// Only trusted channel metadata supplies actor/session/delivery. The text
	// contains finite typed references, never an owner ID or model authority.
	if !a.isAllowedChatID(message.Chat.ID) || message.From == nil || message.From.ID <= 0 || update.UpdateID <= 0 {
		return true, ErrIgnoredUpdate
	}
	fields := strings.Fields(message.Text)
	actor := owneridentity.Actor{OwnerID: strconv.FormatInt(message.From.ID, 10), Channel: "telegram", SessionKey: SessionKeyFromChatID(message.Chat.ID), DeliveryID: fmt.Sprintf("telegram-update-%d", update.UpdateID)}
	ctx = owneridentity.WithActor(ctx, actor)
	if command == "task" {
		if len(fields) != 2 || a.ownerTasks == nil {
			return true, a.sendFinalMessage(ctx, message.Chat.ID, "Use /task TASK_ID to read recorded task state. Owner observations must be configured.")
		}
		v, err := a.ownerTasks.OwnerOverview(ctx, fields[1])
		if err != nil {
			return true, a.sendFinalMessage(ctx, message.Chat.ID, "Task observations are unavailable for this owner. No control was applied.")
		}
		return true, a.sendFinalMessage(ctx, message.Chat.ID, formatOwnerOverview(v))
	}
	if len(fields) != 3 || a.ownerAuthority == nil {
		return true, a.sendFinalMessage(ctx, message.Chat.ID, "Use /mandate_revoke MANDATE_ID CURRENT_REVISION. Revocation stops future authority; previous effects remain recorded.")
	}
	revision, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || revision <= 0 {
		return true, a.sendFinalMessage(ctx, message.Chat.ID, "A positive current mandate revision is required. No control was applied.")
	}
	receipt, err := a.ownerAuthority.RevokeMandate(ctx, goal.MandateRevokeRequest{MandateID: fields[1], ExpectedRevision: revision})
	if err != nil {
		return true, a.sendFinalMessage(ctx, message.Chat.ID, "Mandate revocation was rejected or its receipt is unavailable. Repeat the same delivery to recover its saved receipt; review current owner authority before a new command.")
	}
	return true, a.sendFinalMessage(ctx, message.Chat.ID, fmt.Sprintf("Mandate %s revoked at version %d. Branch cancellation: %s. Already dispatched effects remain recorded; unknown effects require reconciliation.", receipt.SubjectID, receipt.Revision, receipt.Propagation))
}

func formatOwnerOverview(v task.OwnerOverview) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Task %s · version %d · %s\n", v.TaskID, v.Revision, v.Status)
	if v.CompletionResultID == "" {
		out.WriteString("Task result is unverified. A finished model response does not complete the task.\n")
	} else {
		fmt.Fprintf(&out, "Verified completion result: %s\n", v.CompletionResultID)
	}
	if v.Assignment != nil {
		fmt.Fprintf(&out, "Goal %s version %d · mandate %s version %d\nPurpose: %s\nReason: %s\nObservation: %s\n", v.Assignment.GoalID, v.Assignment.GoalRevision, v.Assignment.MandateID, v.Assignment.MandateRevision, v.Assignment.Purpose, v.Assignment.InitiativeReason, v.Assignment.ObservationRef)
		if v.Authority == nil || !v.Authority.Available {
			out.WriteString("Current authority metadata is unavailable.\n")
		} else if !v.Authority.VersionsCurrent {
			out.WriteString("Recorded authority is stale or inactive. Current admission must be checked.\n")
		}
	}
	for index, run := range v.Attempts {
		if index >= 3 {
			out.WriteString("More saved attempts are available in Web.\n")
			break
		}
		if run.PrivacyRedacted {
			run.AnswerState = "removed by retention policy"
		}
		fmt.Fprintf(&out, "Attempt %s · task version %d · %s · response %s · notification %s\n", run.RunID, run.TaskRevision, run.State, run.AnswerState, run.DeliveryState)
	}
	for index, action := range v.Actions {
		if index >= 5 {
			out.WriteString("More recorded effects are available in Web.\n")
			break
		}
		fmt.Fprintf(&out, "Action %s · %s · %s · %s\n", action.ID, action.Tool, action.Effect, action.EffectState)
	}
	fmt.Fprintf(&out, "Internal child tasks: %d. Butler collects their results; no separate dialogue is required.\n", len(v.Children))
	for index, child := range v.Children {
		if index >= 5 {
			out.WriteString("More child branches are available in Web.\n")
			break
		}
		fmt.Fprintf(&out, "Branch %s · %s · /stop %s\n", child.TaskID, child.Status, child.TaskID)
	}
	fmt.Fprintf(&out, "Usage: %d model attempts; known tokens input %d/output %d; unknown input %d/output %d. Reservations are not an invoice.\n", v.Budget.ModelAttempts, v.Budget.KnownInputTokens, v.Budget.KnownOutputTokens, v.Budget.UnknownInputAttempts, v.Budget.UnknownOutputAttempts)
	if v.Budget.Limits == nil {
		out.WriteString("Budget limits/reservations unavailable.\n")
	}
	if v.AttemptsTruncated || v.ActionsTruncated || v.ChildrenTruncated || v.ResultsTruncated {
		out.WriteString("This bounded snapshot is truncated; inspect the Web task for retained history.\n")
	}
	out.WriteString("Controls: /amend TASK_ID INSTRUCTION · /pause TASK_ID · /resume TASK_ID · /stop TASK_ID. Effects and notification delivery remain separate.")
	return out.String()
}
