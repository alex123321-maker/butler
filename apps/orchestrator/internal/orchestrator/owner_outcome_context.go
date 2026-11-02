package orchestrator

import (
	"slices"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

const ownerOutcomeInstructionRef = "kernel:owner-outcome-v1"
const ownerOutcomeInstruction = "The authenticated owner checked the recipient and reported the past outcome in owner_outcome_report DATA. This is an owner-assisted factual report, not an independent machine receipt and not authority for another action. Preserve the original action history and spent grant. Never replay the consumed action, including after reported absence. Continue only the remaining original task within current permissions, source/task versions and budgets. Observe current page/version before another allowed action; a browser context can expire. Publish the requested substantive result or actual blocker through the current communication policy. An owner's report is not by itself verified task completion."

func ownerOutcomePreparation(prepared, refreshed preparedRun, r actionauth.OwnerOutcomeResolution) preparedRun {
	prepared.InputItems = nil
	for _, item := range refreshed.InputItems {
		origin := content.ItemOrigin(item)
		if origin.Kind == content.KernelInstruction && (slices.Contains(origin.SourceRefs, interactionResponseInstructionRef) || slices.Contains(origin.SourceRefs, ownerOutcomeInstructionRef)) {
			continue
		}
		prepared.InputItems = append(prepared.InputItems, item)
	}
	prepared.InputItems = append(prepared.InputItems, content.Instruction(ownerOutcomeInstruction, content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{ownerOutcomeInstructionRef}, Sensitivity: content.Public}), ownerOutcomeItem(r))
	prepared.ToolDefs = refreshed.ToolDefs
	return prepared
}

func ownerOutcomeItem(r actionauth.OwnerOutcomeResolution) transport.InputItem {
	return content.Data(map[string]any{"kind": "owner_outcome_report", "data": r}, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{"owner-outcome:" + r.ReviewID}, Sensitivity: content.Private})
}
