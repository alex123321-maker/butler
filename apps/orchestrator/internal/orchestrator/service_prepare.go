package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
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
	userID := firstString(payload, "user_id", "external_user_id", "author_id")
	if userID == "" {
		userID = event.SessionKey
	}
	channel := strings.TrimSpace(event.Source)
	if channel == "" {
		channel = "unknown"
	}
	prepared := preparedRun{
		InputItems:    []transport.InputItem{{Role: "user", Content: message, ContentType: "text/plain"}},
		UserMessage:   message,
		MemoryBundle:  map[string]any{},
		WorkingMemory: &workingMemoryContext{Status: "idle", Policy: s.config.WorkingPolicy, Scratch: map[string]any{}},
		InputPayload:  payload,
		SessionUserID: userID,
		Channel:       channel,
	}
	if err := s.attachMemoryContext(ctx, event.SessionKey, prepared.SessionUserID, message, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachTranscriptContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachToolContext(ctx, &prepared); err != nil {
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
	bundle, err := s.config.MemoryBundles.BuildBundle(ctx, memoryservice.BundleRequest{
		SessionKey:   sessionKey,
		UserID:       userID,
		UserMessage:  userMessage,
		IncludeQuery: true,
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
	contracts, err := s.config.ToolCatalog.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("load tool contracts: %w", err)
	}
	prepared.ToolDefs = toolDefinitionsFromContracts(contracts)
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
	if strings.TrimSpace(stringFromAny(prepared.MemoryBundle["session_summary"])) != "" {
		return nil
	}
	full, err := s.transcript.GetTranscript(ctx, sessionKey)
	if err != nil {
		return nil
	}
	items := recentTranscriptInputItems(full, 10)
	if len(items) == 0 {
		return nil
	}
	prepared.InputItems = append(items, prepared.InputItems...)
	prepared.observabilityHistory = map[string]any{
		"replayed_message_count": len(items),
		"fallback_used":          true,
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
	prepared.Prompt = s.config.PromptAssembler.Assemble(state, promptmgmt.Context{
		SessionSummary:  stringFromAny(prepared.MemoryBundle["session_summary"]),
		Working:         workingContextToPromptContext(prepared.MemoryBundle["working"]),
		Profile:         sliceOfMaps(prepared.MemoryBundle["profile"]),
		Episodes:        sliceOfMaps(prepared.MemoryBundle["episodes"]),
		Chunks:          sliceOfMaps(prepared.MemoryBundle["chunks"]),
		ToolSummary:     stringFromAny(prepared.MemoryBundle["tool_summary"]),
		BrowserStrategy: promptmgmt.BrowserStrategyContent,
	})
	if strings.TrimSpace(prepared.Prompt.FinalPrompt) != "" {
		prepared.InputItems = append([]transport.InputItem{{Role: "system", Content: prepared.Prompt.FinalPrompt, ContentType: "text/plain"}}, prepared.InputItems...)
	}

	memorySections := make([]string, 0)
	for _, key := range []string{"session_summary", "working", "profile", "episodes", "chunks", "tool_summary"} {
		if _, ok := prepared.MemoryBundle[key]; ok {
			memorySections = append(memorySections, key)
		}
	}
	prepared.observabilityPrompt = map[string]any{
		"has_system_prompt": strings.TrimSpace(prepared.Prompt.FinalPrompt) != "",
		"memory_sections":   memorySections,
		"prompt_length":     len(prepared.Prompt.FinalPrompt),
	}
	return nil
}

func recentTranscriptInputItems(full transcript.Transcript, limit int) []transport.InputItem {
	if limit <= 0 || len(full.Messages) == 0 {
		return nil
	}
	messages := make([]transcript.Message, 0, len(full.Messages))
	for _, msg := range full.Messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(sanitize.TranscriptMessageContent(msg.Content))
		if content == "" {
			continue
		}
		msg.Content = truncateTranscriptReplay(content, 1200)
		messages = append(messages, msg)
	}
	if len(messages) == 0 {
		return nil
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	items := make([]transport.InputItem, 0, len(messages))
	for _, msg := range messages {
		items = append(items, transport.InputItem{Role: strings.ToLower(strings.TrimSpace(msg.Role)), Content: msg.Content, ContentType: "text/plain"})
	}
	return items
}

func truncateTranscriptReplay(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= len("[truncated]") {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-len("[truncated]")-1]) + "\n[truncated]"
}
