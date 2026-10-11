package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) prepareRun(ctx context.Context, event ingress.InputEvent) (preparedRun, error) {
	payload := map[string]any{}
	if strings.TrimSpace(event.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return preparedRun{}, fmt.Errorf("decode input payload: %w", err)
		}
	}
	message := extractUserMessage(payload)
	if strings.TrimSpace(message) == "" {
		message = event.PayloadJSON
	}
	// Query embeddings and working context use the same sanitized owner input.
	message = sanitize.Text(message)
	userID := firstString(payload, "user_id", "external_user_id", "author_id")
	if userID == "" {
		userID = event.SessionKey
	}
	// Memory ownership comes from the durable session, not payload identity.
	if reader, ok := s.sessions.(interface {
		GetSessionByKey(context.Context, string) (session.SessionRecord, error)
	}); ok {
		record, err := reader.GetSessionByKey(ctx, event.SessionKey)
		if err != nil {
			return preparedRun{}, fmt.Errorf("load session ownership: %w", err)
		}
		userID = record.UserID
	}
	channel := strings.TrimSpace(event.Source)
	if channel == "" {
		channel = "unknown"
	}
	prepared := preparedRun{
		InputItems:    []transport.InputItem{content.CurrentOwnerMessage(message, "input:"+event.EventID)},
		UserMessage:   message,
		MemoryBundle:  map[string]any{},
		WorkingMemory: &workingMemoryContext{Status: "idle", Policy: s.config.WorkingPolicy, Scratch: map[string]any{}},
		InputPayload:  payload,
		SessionUserID: userID,
		Channel:       channel,
	}
	if token, ok := ownership.TokenFromContext(ctx); ok {
		prepared.CurrentRunID = token.RunID
	}
	// Pin a validated explicit task before Catalog chooses a delegated context.
	// Later task attachment also prepares working state and authenticated policy.
	if id := firstString(payload, "task_id"); id != "" && prepared.CurrentRunID != "" {
		if s.config.Tasks == nil {
			return preparedRun{}, fmt.Errorf("task service is not configured")
		}
		record, err := s.config.Tasks.Get(ctx, id)
		if err != nil || record.SessionKey != event.SessionKey || record.OwnerID != userID || record.Status != "active" {
			return preparedRun{}, fmt.Errorf("current preparation task authority is required")
		}
		if err = s.config.Tasks.AttachRun(ctx, record.TaskID, record.Revision, prepared.CurrentRunID, event.SessionKey); err != nil {
			return preparedRun{}, err
		}
	}
	delegated, err := s.delegatedContext(ctx, event.SessionKey)
	if err != nil {
		return preparedRun{}, err
	}
	if err = s.attachDelegatedContext(ctx, event, &prepared, delegated); err != nil {
		return preparedRun{}, err
	}
	if delegated == nil {
		if err = s.attachMemoryContext(ctx, event.SessionKey, prepared.SessionUserID, message, &prepared); err != nil {
			return preparedRun{}, err
		}
	}
	if err := s.attachTranscriptContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachTaskContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachToolContext(ctx, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachSkillContext(ctx, &prepared, false); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachPromptContext(ctx, &prepared); err != nil {
		return preparedRun{}, err
	}
	return prepared, nil
}

func (s *Service) attachMemoryContext(ctx context.Context, sessionKey, userID, userMessage string, prepared *preparedRun) error {
	if prepared == nil {
		return nil
	}
	includeQuery := true
	if s.config.OwnerGoals != nil && s.config.Tasks != nil {
		if id := firstString(prepared.InputPayload, "task_id"); id != "" {
			record, err := s.config.Tasks.Get(ctx, id)
			if err != nil || record.SessionKey != sessionKey || record.OwnerID != userID || record.Status != "active" {
				return fmt.Errorf("current memory task authority is required")
			}
			a, m, _, err := s.config.OwnerGoals.ReadTaskAuthority(ctx, id)
			if err != nil {
				return fmt.Errorf("memory data-release authority is unavailable")
			}
			if a.TaskID != "" && m.Privacy != "cloud_allowed" {
				includeQuery = false
			}
		}
	}
	bundle, err := s.config.MemoryBundles.BuildBundle(ctx, memoryservice.BundleRequest{
		SessionKey:   sessionKey,
		UserID:       userID,
		UserMessage:  userMessage,
		IncludeQuery: includeQuery,
	})
	if err != nil {
		return err
	}
	workingMemory := workingMemoryFromBundle(bundle.Working, s.config.WorkingPolicy)
	prepared.WorkingMemory = workingMemory
	if len(bundle.Items) == 0 && workingMemoryIsEmpty(workingMemory) && strings.TrimSpace(bundle.Prompt) == "" {
		return nil
	}
	for key, value := range bundle.Items {
		prepared.MemoryBundle[key] = value
	}

	bundleKeys := make([]string, 0, len(bundle.Items))
	for key := range bundle.Items {
		bundleKeys = append(bundleKeys, key)
	}
	prepared.observabilityMemory = map[string]any{
		"bundle_keys":  bundleKeys,
		"has_working":  !workingMemoryIsEmpty(workingMemory),
		"has_prompt":   strings.TrimSpace(bundle.Prompt) != "",
		"bundle_count": len(bundle.Items),
	}
	return nil
}

func (s *Service) attachToolContext(ctx context.Context, prepared *preparedRun) error {
	if prepared == nil || s.config.ToolCatalog == nil {
		return nil
	}
	var contracts []*toolbrokerv1.ToolContract
	var err error
	if scoped, ok := s.config.ToolCatalog.(interface {
		ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
	}); ok && prepared.CurrentRunID != "" {
		contracts, err = scoped.ListToolsForRun(ctx, prepared.CurrentRunID)
	} else {
		contracts, err = s.config.ToolCatalog.ListTools(ctx)
	}
	if err != nil {
		return fmt.Errorf("load tool contracts: %w", err)
	}
	prepared.ToolDefs = toolDefinitionsFromContracts(contracts)
	for _, contract := range contracts {
		if extension := contract.GetExtension(); extension != nil {
			prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"extension_operation": contract.GetToolName(), "package_description": contract.GetDescription(), "manifest_source": extension.GetSource(), "manifest_version": extension.GetVersion()}, content.Origin{Kind: content.Document, Sensitivity: content.Private, SourceRefs: []string{"extension-manifest:" + extension.GetManifestHash()}}))
		}
	}
	if summary := toolSummaryFromContracts(contracts); strings.TrimSpace(summary) != "" {
		prepared.MemoryBundle["tool_summary"] = summary
	}

	toolNames := make([]string, 0, len(prepared.ToolDefs))
	for _, td := range prepared.ToolDefs {
		toolNames = append(toolNames, td.Name)
	}
	prepared.observabilityTools = map[string]any{
		"tool_count": len(prepared.ToolDefs),
		"tool_names": toolNames,
	}
	return nil
}

func (s *Service) attachTranscriptContext(ctx context.Context, sessionKey string, prepared *preparedRun) error {
	if prepared == nil || s.transcript == nil {
		return nil
	}
	full, err := s.transcript.GetTranscript(ctx, sessionKey)
	if err != nil {
		return fmt.Errorf("load session transcript: %w", err)
	}
	if s.config.MemorySources != nil {
		if err = s.config.MemorySources.PinTranscript(ctx, sessionKey, full.SourceRevision); err != nil {
			return err
		}
	}
	// Check the complete source before summary coverage removes its prefix.
	// A historical secret label cannot be laundered through a covered summary.
	for _, message := range full.Messages {
		if historicalMessageOrigin(message).Sensitivity == content.Secret {
			return content.ErrSecret
		}
	}
	tail := full.Messages
	coverageMode := "full_history_unversioned"
	var throughSequence int64
	var summaryItem *transport.InputItem
	if reader, ok := s.config.SummaryReader.(interface {
		GetSummarySnapshot(context.Context, string) (summary.Snapshot, error)
	}); ok {
		snapshot, err := reader.GetSummarySnapshot(ctx, sessionKey)
		if err != nil {
			return fmt.Errorf("load session summary coverage: %w", err)
		}
		var verified bool
		tail, verified = summary.Tail(full, snapshot)
		// Coverage and summary must come from one checked snapshot, rather than
		// the independently budgeted memory retrieval result.
		delete(prepared.MemoryBundle, "session_summary")
		if verified {
			coverageMode = "verified_summary_and_full_tail"
			throughSequence = snapshot.ThroughSequence
			item := content.Data(map[string]any{"historical_session_summary": sanitize.Text(snapshot.Summary)}, content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{fmt.Sprintf("session:%s:summary:%d:through:%d", sessionKey, snapshot.SourceRevision, snapshot.ThroughSequence)}, Sensitivity: content.Private})
			summaryItem = &item
		} else {
			coverageMode = "full_history_unverified_summary"
		}
	}
	// Validate summary coverage against the complete source first. A recovery
	// reuses its durable input rather than appending or replaying it twice.
	if prepared.CurrentRunID != "" {
		for _, message := range full.Messages {
			if message.RunID == prepared.CurrentRunID && message.Role == "user" {
				prepared.InputAlreadyRecorded = true
			}
		}
		filtered := make([]transcript.Message, 0, len(tail))
		for _, message := range tail {
			if message.RunID != prepared.CurrentRunID || message.Role != "user" {
				filtered = append(filtered, message)
			}
		}
		tail = filtered
	}
	items := transcriptInputItems(tail)
	if summaryItem != nil {
		items = append([]transport.InputItem{*summaryItem}, items...)
	}
	prepared.InputItems = append(items, prepared.InputItems...)
	prepared.observabilityHistory = map[string]any{
		"replayed_message_count": len(items),
		"summary_present":        throughSequence > 0,
		"coverage_mode":          coverageMode,
		"through_sequence":       throughSequence,
		"source_revision":        full.SourceRevision,
	}
	return nil
}

func (s *Service) attachPromptContext(ctx context.Context, prepared *preparedRun) error {
	if prepared == nil || s.config.PromptManager == nil || s.config.PromptAssembler == nil {
		return nil
	}
	state, err := s.config.PromptManager.Get(ctx)
	if err != nil {
		return fmt.Errorf("load prompt config: %w", err)
	}
	assemble := func() promptmgmt.Assembly {
		return s.config.PromptAssembler.Assemble(state, promptmgmt.Context{
			SessionSummary:  stringFromAny(prepared.MemoryBundle["session_summary"]),
			Working:         workingContextToPromptContext(prepared.MemoryBundle["working"]),
			Profile:         sliceOfMaps(prepared.MemoryBundle["profile"]),
			Episodes:        sliceOfMaps(prepared.MemoryBundle["episodes"]),
			Chunks:          sliceOfMaps(prepared.MemoryBundle["chunks"]),
			ToolSummary:     stringFromAny(prepared.MemoryBundle["tool_summary"]),
			BrowserStrategy: promptmgmt.BrowserStrategyContent,
		})
	}
	prepared.Prompt = assemble()
	modelItems := func() []transport.InputItem {
		instructions := prepared.Prompt.TrustedInstructions()
		ref := fmt.Sprintf("operator:base_prompt:%s:sha256:%x", state.Source, sha256.Sum256([]byte(instructions)))
		items := []transport.InputItem{content.Instruction(instructions, content.Origin{Kind: content.OperatorInstruction, SourceRefs: []string{ref}, Sensitivity: content.Private})}
		for _, section := range prepared.Prompt.Sections {
			if section.Omitted || section.Name == "browser_strategy" {
				continue
			}
			kind := content.MemoryDerived
			if section.Name == "document_chunks" {
				kind = content.Document
			}
			if section.Name == "tool_summary" {
				kind = content.ToolOutput
			}
			items = append(items, content.Data(map[string]any{"section": section.Name, "content": section.Content}, content.Origin{Kind: kind, SourceRefs: memorySourceRefs(prepared.MemoryBundle, section.Name), Sensitivity: content.Private}))
		}
		return append(items, prepared.InputItems...)
	}
	removed := []string{}
	// Retrieved material is optional. Preserve the complete transcript tail,
	// covered summary, current message and separately attached task constraints.
	for _, key := range []string{"chunks", "episodes", "profile", "tool_summary"} {
		if estimateContextTokens(modelItems(), prepared.ToolDefs, "") <= s.config.ContextBudgetTokens {
			break
		}
		if _, ok := prepared.MemoryBundle[key]; ok {
			delete(prepared.MemoryBundle, key)
			removed = append(removed, key)
			prepared.Prompt = assemble()
		}
	}
	estimate := estimateContextTokens(modelItems(), prepared.ToolDefs, "")
	if estimate > s.config.ContextBudgetTokens {
		return fmt.Errorf("context budget exceeded: conservative estimate %d, allowance %d; mandatory history and constraints retained", estimate, s.config.ContextBudgetTokens)
	}
	prepared.InputItems = modelItems()

	memorySections := make([]string, 0)
	for _, key := range []string{"session_summary", "working", "profile", "episodes", "chunks", "tool_summary"} {
		if _, ok := prepared.MemoryBundle[key]; ok {
			memorySections = append(memorySections, key)
		}
	}
	prepared.observabilityPrompt = map[string]any{
		"has_system_prompt":         strings.TrimSpace(prepared.Prompt.FinalPrompt) != "",
		"memory_sections":           memorySections,
		"prompt_length":             len(prepared.Prompt.FinalPrompt),
		"context_token_estimate":    estimate,
		"context_token_budget":      s.config.ContextBudgetTokens,
		"context_estimator":         "utf8_json_bytes_plus_protocol_allowance",
		"optional_sections_removed": removed,
	}
	return nil
}

func transcriptInputItems(messages []transcript.Message) []transport.InputItem {
	items := make([]transport.InputItem, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(sanitize.TranscriptMessageContent(msg.Content))
		if content == "" {
			continue
		}
		items = append(items, historicalMessageItem(msg, role, content))
	}
	return items
}

// This conservative estimator charges one token per encoded UTF-8 byte, plus
// a protocol allowance. It is measurable and deliberately does not pretend to
// be the provider's native tokenizer or its billed usage.
func estimateContextTokens(items []transport.InputItem, tools []transport.ToolDefinition, prompt string) int {
	encoded, err := json.Marshal(struct {
		Items  []transport.InputItem
		Tools  []transport.ToolDefinition
		Prompt string
	}{items, tools, prompt})
	if err != nil {
		// An unencodable request cannot receive a small, successful estimate.
		return int(^uint(0) >> 1)
	}
	return len(encoded) + 1024 + 32*(len(items)+len(tools))
}
