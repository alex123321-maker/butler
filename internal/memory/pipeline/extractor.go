package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/memory/transcript"
)

// ExtractionResult holds the structured output of LLM-based memory extraction.
type ExtractionResult struct {
	ProfileUpdates []ProfileCandidate  `json:"profile_updates"`
	Episodes       []EpisodeCandidate  `json:"episodes"`
	WorkingUpdates []WorkingCandidate  `json:"working_updates"`
	DocumentChunks []DocumentCandidate `json:"document_chunks"`
	SessionSummary string              `json:"session_summary"`
}

// ProfileCandidate represents a candidate profile memory entry from extraction.
type ProfileCandidate struct {
	ScopeType  string  `json:"scope_type"`
	ScopeID    string  `json:"scope_id"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

// EpisodeCandidate represents a candidate episodic memory entry from extraction.
type EpisodeCandidate struct {
	ScopeType  string  `json:"scope_type"`
	ScopeID    string  `json:"scope_id"`
	Summary    string  `json:"summary"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
}

type WorkingCandidate struct {
	ScopeType  string  `json:"scope_type"`
	ScopeID    string  `json:"scope_id"`
	Goal       string  `json:"goal"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

type DocumentCandidate struct {
	ScopeType  string  `json:"scope_type"`
	ScopeID    string  `json:"scope_id"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
}

// Extractor calls an LLM to extract memory candidates from a run transcript.
type Extractor interface {
	Extract(ctx context.Context, sessionKey string, t transcript.Transcript) (*ExtractionResult, error)
}

// LLMCaller abstracts a single LLM text completion call.
type LLMCaller interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// LLMExtractor uses an LLM to extract memory candidates from transcripts.
type LLMExtractor struct {
	llm LLMCaller
}

// NewLLMExtractor creates a new LLM-based memory extractor.
func NewLLMExtractor(llm LLMCaller) *LLMExtractor {
	return &LLMExtractor{llm: llm}
}

const extractionSystemPrompt = `You are a memory extraction agent. Analyze the conversation transcript and extract:

1. Profile updates: facts about the user that should be remembered long-term (preferences, personal info, recurring patterns, system facts).
   - Each profile entry has: scope_type (one of "user", "session", "global"), scope_id, key, value, summary, confidence (0.0-1.0).
   - Use scope_type "user" for personal facts, "session" for session-specific state, "global" for system-wide facts.

2. Episodes: important events or outcomes worth remembering for future reference.
   - Each episode has: scope_type, scope_id, summary, content (detailed description), confidence (0.0-1.0).

3. Working updates: short-lived task state that should stay separate from long-term profile memory.
   - Each working update has: scope_type, scope_id, goal, summary, confidence (0.0-1.0).

4. Document chunks: reusable technical references or compact knowledge snippets if present.
   - Each document chunk has: scope_type, scope_id, title, content, confidence (0.0-1.0).

5. Session summary: a concise summary of the session state after this run, covering: current goal, recent events, open tasks, critical facts.

Return ONLY valid JSON matching this schema:
{
  "profile_updates": [{"scope_type":"...","scope_id":"...","key":"...","value":"...","summary":"...","confidence":0.9}],
  "episodes": [{"scope_type":"...","scope_id":"...","summary":"...","content":"...","confidence":0.8}],
  "working_updates": [{"scope_type":"...","scope_id":"...","goal":"...","summary":"...","confidence":0.8}],
  "document_chunks": [{"scope_type":"...","scope_id":"...","title":"...","content":"...","confidence":0.7}],
  "session_summary": "..."
}

Rules:
- Only extract genuinely important information, not trivial conversational filler.
- Confidence should reflect how certain you are the fact is worth remembering.
- Use the session key as scope_id for session-scoped items.
- Use the user ID as scope_id for user-scoped items.
- Use "global" as scope_id for global items.
- If nothing meaningful is found, return empty arrays and an empty summary.`

// Extract processes a transcript and returns memory extraction candidates.
func (e *LLMExtractor) Extract(ctx context.Context, sessionKey string, t transcript.Transcript) (*ExtractionResult, error) {
	userPrompt := formatTranscriptForExtraction(sessionKey, t)
	if strings.TrimSpace(userPrompt) == "" {
		return &ExtractionResult{}, nil
	}

	response, err := e.llm.Complete(ctx, extractionSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("llm extraction call: %w", err)
	}

	result, err := parseExtractionResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse extraction response: %w", err)
	}
	return result, nil
}

func formatTranscriptForExtraction(sessionKey string, t transcript.Transcript) string {
	if len(t.Messages) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Session: %s\n\n", sessionKey))
	for _, msg := range t.Messages {
		b.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, sanitize.TranscriptMessageContent(msg.Content)))
	}
	if len(t.ToolCalls) > 0 {
		b.WriteString("\nTool calls:\n")
		for _, tc := range t.ToolCalls {
			b.WriteString(fmt.Sprintf("- %s (status: %s) args=%s result=%s error=%s\n", tc.ToolName, tc.Status, sanitize.TranscriptToolArgsJSON(tc.ArgsJSON), sanitize.TranscriptToolResultJSON(tc.ResultJSON), sanitize.TranscriptToolErrorJSON(tc.ErrorJSON)))
		}
	}
	return b.String()
}

func parseExtractionResponse(response string) (*ExtractionResult, error) {
	response = strings.TrimSpace(response)

	// Strip markdown code fences if present.
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		start, end := 0, len(lines)
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				if start == 0 {
					start = i + 1
				} else {
					end = i
					break
				}
			}
		}
		response = strings.Join(lines[start:end], "\n")
	}

	var result ExtractionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("json decode: %w (response: %s)", err, truncate(response, 200))
	}
	return &result, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
