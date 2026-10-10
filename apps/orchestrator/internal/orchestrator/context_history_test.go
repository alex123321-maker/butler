package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
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

func TestTranscriptReplayTruncationPreservesUnicode(t *testing.T) {
	value := strings.Repeat("Привет🦉", 300)
	for _, limit := range []int{1, 2, 12, 1200} {
		got := truncateTranscriptReplay(value, limit)
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) > limit {
			t.Fatalf("limit %d: malformed or oversized replay", limit)
		}
	}
	items := recentTranscriptInputItems(transcript.Transcript{Messages: []transcript.Message{{Role: "assistant", Content: value}}}, 10)
	if len(items) != 1 || !utf8.ValidString(items[0].Content) || utf8.RuneCountInString(items[0].Content) > 1200 {
		t.Fatal("assistant replay limit did not preserve Unicode")
	}
}
