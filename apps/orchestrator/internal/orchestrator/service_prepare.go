package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/toolcatalog"
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
	if operatorDoctor(ctx) {
		if firstString(payload, "task_id") == "" {
			return preparedRun{}, fmt.Errorf("operator diagnostic task required")
		}
		if err := s.attachTaskContext(ctx, event.SessionKey, &prepared); err != nil {
			return preparedRun{}, err
		}
		return prepared, nil
	}
	if s.config.AdaptiveRetrieval != nil {
		plan := adaptive.Classify(message)
		prepared.AdaptivePlan = &plan
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
	if err = s.attachConverterContinuation(ctx, event, &prepared); err != nil {
		return preparedRun{}, err
	}
	if delegated == nil {
		if err = s.attachMemoryContext(ctx, event.SessionKey, prepared.SessionUserID, message, &prepared); err != nil {
			return preparedRun{}, err
		}
	}
	if err := s.attachTaskContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachTranscriptContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if isCurrentOwnerRequest(event, prepared) {
		prepared.CurrentRequest = &currentRequestContext{RunRef: prepared.CurrentRunID, InputEventRef: "input:" + event.EventID}
		attachCurrentRequestContext(&prepared, "")
	}
	if event.Source == "schedule" && prepared.Task != nil && prepared.Task.Source["type"] == "scheduled_job" {
		prepared.InputItems = append(prepared.InputItems, content.Instruction(researchInstruction+"\nThis is a scheduled occurrence, not a new owner message. For a checked monitor finish by calling communication.message with observation and stable event_key. pending/unchanged returns unchanged without a notification. unknown must honestly explain inability to verify; confirmed/postponed require fresh primary evidence. Do not skip recording an observation, create another schedule, ask optional questions or send progress notices. Runtime clock: "+time.Now().UTC().Format(time.RFC3339), content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"kernel:scheduled-research-v1"}, Sensitivity: content.Public}))
	}
	if prepared.AdaptivePlan != nil {
		if err := s.config.AdaptiveRetrieval.Init(ctx, *prepared.AdaptivePlan); err != nil {
			return preparedRun{}, err
		}
		if prepared.AdaptivePlan.SearchEnabled() {
			prepared.InputItems = append(prepared.InputItems, content.Instruction(adaptive.Instruction, content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{adaptive.Version}, Sensitivity: content.Public}))
		}
		metadata, _ := json.Marshal(prepared.AdaptivePlan)
		prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"retrieval_plan": json.RawMessage(metadata)}, content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{adaptive.Version}, Sensitivity: content.Private}))
	}
	if err := s.attachFailedDelegationAttempts(ctx, &prepared); err != nil {
		return preparedRun{}, err
	}
	if s.config.CommunicationAdmission != nil {
		if s.config.Communication == nil {
			return preparedRun{}, fmt.Errorf("communication admission requires owning kernel")
		}
		if err := s.config.CommunicationAdmission(ctx, prepared.CurrentRunID); err != nil {
			return preparedRun{}, err
		}
		if reader, ok := s.config.Communication.(interface {
			ContinuedResponse(context.Context, string) (string, string, error)
		}); ok {
			id, answer, err := reader.ContinuedResponse(ctx, prepared.CurrentRunID)
			if err != nil {
				return preparedRun{}, err
			}
			if id != "" {
				prepared = interactionResponsePreparation(prepared, prepared, id, answer)
				attachCurrentRequestContext(&prepared, id)
			}
		}
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
	policy, authority, profileGoal, err := s.memoryQueryPolicy(ctx, sessionKey, userID, prepared)
	if err != nil {
		return err
	}
	bundle, err := s.config.MemoryBundles.BuildBundle(ctx, memoryservice.BundleRequest{
		SessionKey:         sessionKey,
		UserID:             userID,
		UserMessage:        userMessage,
		ProfileTaskGoal:    profileGoal,
		IncludeQuery:       policy.External, // compatibility only; QueryPolicy is authoritative
		QueryPolicy:        &policy,
		SkipOptionalSearch: prepared.AdaptivePlan != nil,
	})
	if err != nil {
		return err
	}
	_, current, _, err := s.memoryQueryPolicy(ctx, sessionKey, userID, prepared)
	if err != nil || current != authority {
		return fmt.Errorf("memory task authority changed during retrieval")
	}
	if s.config.MemorySources != nil && prepared.CurrentRunID != "" {
		if err = s.config.MemorySources.CheckRunSources(ctx, prepared.CurrentRunID); err != nil {
			return err
		}
	}
	workingMemory := workingMemoryFromBundle(bundle.Working, s.config.WorkingPolicy)
	prepared.WorkingMemory = workingMemory
	for key, value := range bundle.Items {
		prepared.MemoryBundle[key] = value
	}

	bundleKeys := make([]string, 0, len(bundle.Items))
	for key := range bundle.Items {
		bundleKeys = append(bundleKeys, key)
	}
	prepared.observabilityMemory = map[string]any{
		"bundle_keys":            bundleKeys,
		"has_working":            !workingMemoryIsEmpty(workingMemory),
		"has_prompt":             strings.TrimSpace(bundle.Prompt) != "",
		"bundle_count":           len(bundle.Items),
		"query_local_allowed":    policy.Local,
		"query_external_allowed": policy.External,
		"context_selection":      bundle.Diagnostics,
	}
	return nil
}

func (s *Service) memoryQueryPolicy(ctx context.Context, sessionKey, userID string, prepared *preparedRun) (memoryservice.QueryPolicy, string, string, error) {
	policy := memoryservice.QueryPolicy{Local: true, External: true}
	if s.config.OwnerGoals == nil || s.config.Tasks == nil {
		return policy, "", "", nil
	}
	id := firstString(prepared.InputPayload, "task_id")
	if id == "" {
		return policy, "", "", nil
	}
	record, err := s.config.Tasks.Get(ctx, id)
	if err != nil || record.TaskID != id || record.SessionKey != sessionKey || record.OwnerID != userID || record.Status != "active" {
		return policy, "", "", fmt.Errorf("current memory task authority is required")
	}
	a, m, _, err := s.config.OwnerGoals.ReadTaskAuthority(ctx, id)
	if err != nil {
		return policy, "", "", fmt.Errorf("memory data-release authority is unavailable")
	}
	if a.TaskID != "" && m.Privacy != "cloud_allowed" {
		policy.External = false
	}
	stamp, _ := json.Marshal(struct {
		TaskRevision, ConstraintsRevision int64
		Goal                              string
		Assignment                        any
		MandateID                         string
		MandateRevision                   int64
		Privacy                           string
	}{record.Revision, record.ConstraintsRevision, record.Goal, a, m.ID, m.Revision, m.Privacy})
	// Only this validated current task can guide profile ties. Never inherit a
	// goal from payload text, retrieved documents or session working memory.
	return policy, string(stamp), sanitize.Text(record.Goal), nil
}

func (s *Service) attachToolContext(ctx context.Context, prepared *preparedRun) error {
	s.attachMemoryEvidenceInstruction(prepared)
	if s.config.ToolDiscoveryEnabled && s.config.ToolCatalog == nil {
		return fmt.Errorf("tool discovery requires a scoped Broker catalog")
	}
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
	if !s.config.ToolDiscoveryEnabled {
		filtered := contracts[:0]
		for _, c := range contracts {
			if !toolcatalog.IsTool(c.ToolName) {
				filtered = append(filtered, c)
			}
		}
		contracts = filtered
	}
	if prepared.AdaptivePlan != nil && !prepared.AdaptivePlan.SearchEnabled() {
		filtered := contracts[:0]
		for _, contract := range contracts {
			if !readcontrol.IsTool(contract.GetToolName()) {
				filtered = append(filtered, contract)
			}
		}
		contracts = filtered
	}
	if s.config.ToolDiscoveryEnabled {
		categories := map[string]bool{}
		for _, c := range contracts {
			if c.ToolClass != "tool_catalog" {
				categories[c.ToolClass] = true
			}
		}
		categoryNames := []string{}
		for name := range categories {
			categoryNames = append(categoryNames, name)
		}
		sort.Strings(categoryNames)
		prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"available_tool_categories": categoryNames}, content.Origin{Kind: content.ToolOutput, Sensitivity: content.Private, SourceRefs: []string{"tool-catalog:" + prepared.CurrentRunID}}))
		var stale []string
		contracts, stale, err = s.selectToolContracts(ctx, prepared.CurrentRunID, contracts)
		if err != nil {
			return err
		}
		prepared.InputItems = append(prepared.InputItems, discoveryInstructions())
		if len(stale) > 0 {
			prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"unloaded_stale_tool_refs": stale}, content.Origin{Kind: content.ToolOutput, Sensitivity: content.Private, SourceRefs: []string{"tool-catalog:" + prepared.CurrentRunID}}))
		}
	}
	prepared.ToolDefs, err = s.modelToolDefinitions(contracts)
	if err != nil {
		return err
	}
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
		"tool_count":   len(prepared.ToolDefs),
		"tool_names":   toolNames,
		"schema_bytes": len(mustMarshalJSON(prepared.ToolDefs)),
		"deferred":     s.config.ToolDiscoveryEnabled,
	}
	return nil
}

func (s *Service) attachTranscriptContext(ctx context.Context, sessionKey string, prepared *preparedRun) error {
	if prepared == nil || s.transcript == nil {
		return nil
	}
	// Mobile batches share delivery/session identity, but each task owns a finite
	// processing context. Ordinary conversations retain complete session replay.
	if prepared.Task != nil && prepared.Task.Source["type"] == "initiative" && prepared.Task.Source["mobile_batch_id"] != nil {
		reader, ok := s.transcript.(interface {
			GetTaskTranscript(context.Context, string, string) (transcript.Transcript, error)
		})
		if !ok {
			return fmt.Errorf("mobile task transcript reader unavailable")
		}
		full, err := reader.GetTaskTranscript(ctx, sessionKey, prepared.Task.TaskID)
		if err != nil {
			return fmt.Errorf("load mobile task transcript: %w", err)
		}
		if s.config.MemorySources != nil {
			if err = s.config.MemorySources.PinTranscript(ctx, sessionKey, full.SourceRevision); err != nil {
				return err
			}
		}
		for _, message := range full.Messages {
			if historicalMessageOrigin(message).Sensitivity == content.Secret {
				return content.ErrSecret
			}
		}
		messages := full.Messages[:0]
		for _, m := range full.Messages {
			if prepared.CurrentRunID != "" && m.RunID == prepared.CurrentRunID && m.Role == "user" {
				prepared.InputAlreadyRecorded = true
				continue
			}
			messages = append(messages, m)
		}
		items := transcriptInputItems(messages)
		prepared.InputItems = append(items, prepared.InputItems...)
		prepared.observabilityHistory = map[string]any{"replayed_message_count": len(items), "coverage_mode": "mobile_task_history", "source_revision": full.SourceRevision}
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
	if reader, ok := s.config.Communication.(interface {
		ConversationHistory(context.Context, string) ([]string, bool, error)
	}); ok {
		ids, bounded, err := reader.ConversationHistory(ctx, prepared.CurrentRunID)
		if err != nil {
			return fmt.Errorf("load conversation context boundary: %w", err)
		}
		if bounded {
			prepared.InputItems = append(prepared.InputItems,
				content.Instruction(researchInstruction, content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"kernel:current-request-v2"}, Sensitivity: content.Public}),
				content.Instruction("Runtime clock: "+time.Now().UTC().Format(time.RFC3339)+" (UTC).", content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"kernel:runtime-clock-v1"}, Sensitivity: content.Public}))
			if examples, origin := publishedWorkflowExamples(full, ids); len(examples) > 0 {
				prepared.InputItems = append(prepared.InputItems, content.Data(map[string]any{"recent_workflow_examples": examples, "verified_skill_evidence": false}, origin))
			}
			messages, recorded := conversationMessages(full.Messages, prepared.CurrentRunID, ids)
			prepared.InputAlreadyRecorded = recorded
			delete(prepared.MemoryBundle, "session_summary")
			items := transcriptInputItems(messages)
			prepared.InputItems = append(items, prepared.InputItems...)
			prepared.observabilityHistory = map[string]any{"replayed_message_count": len(items), "coverage_mode": "published_exchanges_v1", "source_revision": full.SourceRevision}
			return nil
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
	// Reserve conservative allowance units for answer and tool continuation.
	// Small explicit input allowances retain proportional headroom.
	reserve := min(4096, s.config.ContextBudgetTokens/8)
	allowance := s.config.ContextBudgetTokens - reserve
	// Retrieved material is optional. Preserve the complete transcript tail,
	// covered summary, current message and separately attached task constraints.
	for estimateContextTokens(modelItems(), prepared.ToolDefs, "") > allowance {
		key, ok := removeOptionalMemoryItem(prepared.MemoryBundle)
		if !ok {
			break
		}
		removed = append(removed, key)
		prepared.Prompt = assemble()
	}
	estimate := estimateContextTokens(modelItems(), prepared.ToolDefs, "")
	if estimate > allowance {
		return fmt.Errorf("context budget exceeded: conservative estimate %d, allowance %d; mandatory history and constraints retained", estimate, allowance)
	}
	prepared.InputItems = modelItems()

	memorySections := make([]string, 0)
	for _, key := range []string{"session_summary", "working", "profile", "episodes", "chunks", "tool_summary"} {
		if _, ok := prepared.MemoryBundle[key]; ok {
			memorySections = append(memorySections, key)
		}
	}
	prepared.observabilityPrompt = map[string]any{
		"has_system_prompt":          strings.TrimSpace(prepared.Prompt.FinalPrompt) != "",
		"memory_sections":            memorySections,
		"prompt_length":              len(prepared.Prompt.FinalPrompt),
		"context_token_estimate":     estimate,
		"context_token_budget":       s.config.ContextBudgetTokens,
		"continuation_reserve_units": reserve,
		"context_estimator":          "utf8_json_bytes_plus_protocol_allowance",
		"optional_sections_removed":  removed,
	}
	return nil
}

func removeOptionalMemoryItem(bundle map[string]any) (string, bool) {
	for _, key := range []string{"tool_summary", "profile", "episodes", "chunks"} {
		value, ok := bundle[key]
		if !ok {
			continue
		}
		entries, ok := value.([]map[string]any)
		if !ok || len(entries) <= 1 {
			delete(bundle, key)
		} else {
			bundle[key] = entries[:len(entries)-1]
		}
		return key, true
	}
	return "", false
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
