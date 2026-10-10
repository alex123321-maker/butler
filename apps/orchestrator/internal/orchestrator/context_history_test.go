package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func TestSummaryCannotHideRecentOwnerCorrection(t *testing.T) {
	for _, providerName := range []string{"openai", "github_copilot"} {
		for _, summary := range []string{"", "Можно отправлять автоматически.", "Теперь только черновики."} {
			t.Run(providerName+"/"+summary, func(t *testing.T) {
				const sessionKey = "web:correction"
				correction := strings.Repeat("Обсуждаем проект. ", 180) + "Для этого проекта теперь только черновики; ничего не отправляй."
				store := &memoryTranscriptStore{messages: []transcript.Message{
					{SessionKey: sessionKey, Role: "assistant", Content: "Можно отправить автоматически."},
					{SessionKey: sessionKey, Role: "user", Content: correction},
				}}
				provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", providerName, transport.AssistantFinal{Content: "Черновик."})}}
				service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), store, provider, Config{
					ProviderName:  providerName,
					SummaryReader: &stubSummaryReader{summaries: map[string]string{sessionKey: summary}},
				}, nil)
				_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "correction", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: sessionKey, Source: "web", PayloadJSON: `{"text":"Продолжай"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "correction"})
				if err != nil {
					t.Fatal(err)
				}
				request := provider.startRequests[0]
				if request.Context.ProviderSessionRef != nil {
					t.Fatal("history replay reused provider state")
				}
				count := 0
				for _, item := range request.InputItems {
					if item.Role == "user" && item.Content == correction {
						count++
					}
					if !utf8.ValidString(item.Content) {
						t.Fatal("invalid UTF-8 in context")
					}
				}
				if count != 1 {
					t.Fatalf("fresh correction must appear intact exactly once, got %d", count)
				}
			})
		}
	}
}

type coveredHistoryStore struct {
	*memoryTranscriptStore
	revision int64
}

func (s *coveredHistoryStore) GetTranscript(ctx context.Context, key string) (transcript.Transcript, error) {
	source, err := s.memoryTranscriptStore.GetTranscript(ctx, key)
	source.SourceRevision = s.revision
	return source, err
}

type coveredSummaryReader struct {
	snapshot summary.Snapshot
	err      error
}

func (r coveredSummaryReader) GetSummary(context.Context, string) (string, error) {
	return r.snapshot.Summary, r.err
}
func (r coveredSummaryReader) GetSummarySnapshot(context.Context, string) (summary.Snapshot, error) {
	return r.snapshot, r.err
}

func TestCoveredSummaryReplaysOnlyCompleteUncompressedTail(t *testing.T) {
	for _, providerName := range []string{"openai", "github_copilot"} {
		for _, mode := range []string{"empty", "old", "current", "future", "edited"} {
			t.Run(providerName+"/"+mode, func(t *testing.T) {
				const key = "web:coverage"
				correction := strings.Repeat("Длинное обсуждение 🦉. ", 180) + "Теперь только черновики; ничего не отправляй."
				messages := []transcript.Message{{SessionKey: key, MessageID: "a", Sequence: 1, Role: "assistant", Content: "Можно отправлять автоматически."}}
				// More than ten messages prevents regression to a fixed recent window.
				for i := 2; i < 15; i++ {
					messages = append(messages, transcript.Message{SessionKey: key, MessageID: fmt.Sprintf("m%d", i), Sequence: int64(i), Role: "assistant", Content: strings.Repeat("Ответ. ", 220)})
				}
				messages = append(messages, transcript.Message{SessionKey: key, MessageID: "correction", Sequence: 15, Role: "user", Content: correction})
				snapshot := summary.Snapshot{SessionKey: key, Summary: "Можно отправлять автоматически.", ThroughMessageID: "a", ThroughSequence: 1, SourceRevision: 3, FormatVersion: 1}
				switch mode {
				case "empty":
					snapshot = summary.Snapshot{}
				case "current":
					snapshot.ThroughMessageID = "m14"
					snapshot.ThroughSequence = 14
				case "future":
					snapshot.ThroughMessageID = "future"
					snapshot.ThroughSequence = 16
				case "edited":
					snapshot.SourceRevision = 2
				}
				store := &coveredHistoryStore{memoryTranscriptStore: &memoryTranscriptStore{messages: messages}, revision: 3}
				provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", providerName, transport.AssistantFinal{Content: "Черновик"})}}
				service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), store, provider, Config{ProviderName: providerName, SummaryReader: coveredSummaryReader{snapshot: snapshot}}, nil)
				_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "coverage", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: key, Source: "web", PayloadJSON: `{"text":"Продолжай"}`, CreatedAt: time.Now(), IdempotencyKey: "coverage"})
				if err != nil {
					t.Fatal(err)
				}
				req := provider.startRequests[0]
				if req.Context.ProviderSessionRef != nil {
					t.Fatal("provider state duplicated Butler history")
				}
				corrections, historyCount := 0, 0
				for _, item := range req.InputItems {
					if !utf8.ValidString(item.Content) {
						t.Fatal("invalid UTF-8")
					}
					if item.Content == correction {
						corrections++
					}
					if item.Role == "assistant" {
						historyCount++
						if item.Content == strings.TrimSpace(strings.Repeat("Ответ. ", 220)) {
							continue
						}
						if item.Content != messages[0].Content {
							t.Fatal("tail assistant text was compressed")
						}
					}
				}
				if corrections != 1 {
					t.Fatalf("correction count=%d", corrections)
				}
				want := 14
				if mode == "old" {
					want = 13
				}
				if mode == "current" {
					want = 0
				}
				if historyCount != want {
					t.Fatalf("history count=%d want %d", historyCount, want)
				}
			})
		}
	}
}

func TestSummaryCoverageReadFailureBlocksModel(t *testing.T) {
	provider := &mockProvider{}
	service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{SummaryReader: coveredSummaryReader{err: errors.New("coverage unavailable")}}, nil)
	_, err := service.prepareRun(context.Background(), ingress.InputEvent{SessionKey: "s", PayloadJSON: `{"text":"continue"}`})
	if err == nil || !strings.Contains(err.Error(), "load session summary coverage") || len(provider.startRequests) != 0 {
		t.Fatalf("coverage read failure: %v", err)
	}
}

func TestPreparationRecoveryKeepsVerifiedFullTailAndCurrentInputOnce(t *testing.T) {
	const key = "web:preparation-recovery"
	for _, covered := range []bool{false, true} {
		t.Run(fmt.Sprint(covered), func(t *testing.T) {
			messages := []transcript.Message{{SessionKey: key, MessageID: "boundary", Sequence: 1, Role: "assistant", Content: "covered text"}}
			for i := 2; i <= 15; i++ {
				messages = append(messages, transcript.Message{SessionKey: key, MessageID: fmt.Sprint(i), Sequence: int64(i), Role: "assistant", Content: fmt.Sprintf("complete prior response %d", i)})
			}
			messages = append(messages, transcript.Message{SessionKey: key, RunID: "current", MessageID: "input", Sequence: 16, Role: "user", Content: "current input"})
			snapshot := summary.Snapshot{}
			if covered {
				snapshot = summary.Snapshot{SessionKey: key, Summary: "verified old summary", ThroughMessageID: "boundary", ThroughSequence: 1, SourceRevision: 7, FormatVersion: 1}
			}
			store := &coveredHistoryStore{memoryTranscriptStore: &memoryTranscriptStore{messages: messages}, revision: 7}
			service := NewService(nil, nil, nil, store, nil, Config{SummaryReader: coveredSummaryReader{snapshot: snapshot}}, nil)
			prepared := preparedRun{CurrentRunID: "current", InputItems: []transport.InputItem{{Role: "user", Content: "current input"}}, MemoryBundle: map[string]any{}}
			if err := service.attachTranscriptContext(context.Background(), key, &prepared); err != nil {
				t.Fatal(err)
			}
			if !prepared.InputAlreadyRecorded || len(prepared.InputItems) != 16 {
				t.Fatalf("lost history or repeated input: %#v", prepared)
			}
			users, assistants := 0, 0
			for _, item := range prepared.InputItems {
				if item.Role == "user" && item.Content == "current input" {
					users++
				}
				if item.Role == "assistant" {
					assistants++
				}
			}
			wantAssistants := 15
			if covered {
				wantAssistants = 14
				if prepared.observabilityHistory["coverage_mode"] != "verified_summary_and_full_tail" {
					t.Fatal("recovery filtering invalidated legitimate summary coverage")
				}
			}
			if users != 1 || assistants != wantAssistants {
				t.Fatalf("users=%d assistants=%d", users, assistants)
			}
		})
	}
}

func TestContextBudgetDropsOptionalMaterialBeforeMandatoryConstraints(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil, Config{}, nil)
	mandatory := transport.InputItem{Role: "system", Content: strings.Repeat("Точное ограничение. ", 100) + "НЕ ОТПРАВЛЯТЬ"}
	prepared := preparedRun{InputItems: []transport.InputItem{mandatory, {Role: "user", Content: "продолжай"}}, MemoryBundle: map[string]any{"profile": []map[string]any{{"key": "background", "summary": strings.Repeat("Второстепенное. ", 300)}}}}
	// Measure the mandatory-only baseline through the same assembly path.
	baseline := prepared
	baseline.MemoryBundle = map[string]any{}
	if err := service.attachPromptContext(context.Background(), &baseline); err != nil {
		t.Fatal(err)
	}
	service.config.ContextBudgetTokens = baseline.observabilityPrompt["context_token_estimate"].(int) + 16
	if err := service.attachPromptContext(context.Background(), &prepared); err != nil {
		t.Fatal(err)
	}
	if _, ok := prepared.MemoryBundle["profile"]; ok {
		t.Fatal("optional profile displaced protected instructions")
	}
	if prepared.InputItems[1].Content != mandatory.Content {
		t.Fatal("protected constraints truncated")
	}
	service.config.ContextBudgetTokens = 100
	_, err := service.prepareRun(context.Background(), ingress.InputEvent{SessionKey: "s", PayloadJSON: `{"text":"Не отправляй"}`})
	if err == nil || !strings.Contains(err.Error(), "context budget exceeded") {
		t.Fatalf("oversized mandatory context silently dropped: %v", err)
	}
}

func TestHistoryReadFailureBlocksModel(t *testing.T) {
	for _, summary := range []string{"", "Old summary"} {
		provider := &mockProvider{}
		service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{failHistory: errors.New("history unavailable")}, provider, Config{
			SummaryReader: &stubSummaryReader{summaries: map[string]string{"web:1": summary}},
		}, nil)
		_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "history", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:1", Source: "web", PayloadJSON: `{"text":"continue"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "history"})
		if err == nil || !strings.Contains(err.Error(), "load session transcript") {
			t.Fatalf("history failure was indistinguishable from empty history: %v", err)
		}
		if len(provider.startRequests) != 0 {
			t.Fatal("model started with unverifiable history")
		}
	}
}

func TestTranscriptReplayPreservesCompleteUnicodeTail(t *testing.T) {
	value := strings.Repeat("Привет🦉", 300)
	items := transcriptInputItems([]transcript.Message{{Role: "assistant", Content: value}})
	if len(items) != 1 || !utf8.ValidString(items[0].Content) || items[0].Content != value {
		t.Fatal("unsummarized assistant message was abbreviated or corrupted")
	}
}
