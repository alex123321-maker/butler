package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
)

type coveredTestStore struct {
	current summary.Snapshot
	writes  []summary.Snapshot
}

func (s *coveredTestStore) UpdateSummary(context.Context, string, string) error {
	return fmt.Errorf("unversioned write forbidden")
}
func (s *coveredTestStore) GetSummarySnapshot(context.Context, string) (summary.Snapshot, error) {
	return s.current, nil
}
func (s *coveredTestStore) SaveCoveredSummary(_ context.Context, snapshot summary.Snapshot) (bool, error) {
	s.writes = append(s.writes, snapshot)
	return true, nil
}

type sessionTestReader struct{ source transcript.Transcript }

func (r sessionTestReader) GetTranscript(context.Context, string) (transcript.Transcript, error) {
	return r.source, nil
}
func (r sessionTestReader) GetRunTranscript(context.Context, string) (transcript.Transcript, error) {
	return r.source, nil
}

type recordingSummarizer struct {
	previous string
	messages []transcript.Message
}

func (g *recordingSummarizer) SummarizeSession(_ context.Context, _ string, previous string, messages []transcript.Message) (string, error) {
	g.previous = previous
	g.messages = messages
	return "Новая сводка", nil
}

func TestPipelinePublishesOnlyVerifiedCompleteSessionCoverage(t *testing.T) {
	for _, mode := range []string{"empty", "old", "current", "edited"} {
		t.Run(mode, func(t *testing.T) {
			source := transcript.Transcript{SourceRevision: 2, Messages: []transcript.Message{{SessionKey: "s", MessageID: "a", Sequence: 1, Role: "user", Content: "Можно отправлять"}, {SessionKey: "s", MessageID: "b", Sequence: 2, Role: "user", Content: "Только черновики"}}}
			store := &coveredTestStore{current: summary.Snapshot{SessionKey: "s", Summary: "Предыдущая сводка", ThroughMessageID: "a", ThroughSequence: 1, SourceRevision: 2, FormatVersion: 1}}
			switch mode {
			case "empty":
				store.current = summary.Snapshot{}
			case "current":
				store.current.ThroughMessageID = "b"
				store.current.ThroughSequence = 2
			case "edited":
				store.current.SourceRevision = 1
			}
			generator := &recordingSummarizer{}
			worker := NewWorker(nil, sessionTestReader{source}, nil, nil, nil, nil, store, WorkerConfig{}, nil, nil)
			worker.SetSessionSummarizer(generator)
			if err := worker.updateSessionSummary(context.Background(), slog.Default(), &Job{SessionKey: "s"}, "This run alone is not full session coverage"); err != nil {
				t.Fatal(err)
			}
			if mode == "current" {
				if len(store.writes) != 0 || generator.messages != nil {
					t.Fatal("already covered source inferred again")
				}
				return
			}
			want := 2
			if mode == "old" {
				want = 1
				if generator.previous != store.current.Summary {
					t.Fatal("previous verified prefix not supplied")
				}
			} else if generator.previous != "" {
				t.Fatal("stale summary reused")
			}
			if len(generator.messages) != want || len(store.writes) != 1 {
				t.Fatal("summarizer did not receive complete tail")
			}
			written := store.writes[0]
			if written.ThroughMessageID != "b" || written.ThroughSequence != 2 || written.SourceRevision != 2 || written.FormatVersion != 1 {
				t.Fatalf("incorrect provenance: %+v", written)
			}
		})
	}
}

func TestSummaryOversizedInputNeverClaimsCoverage(t *testing.T) {
	caller := &fakeLLMCaller{response: "summary"}
	_, err := NewLLMExtractor(caller).SummarizeSession(context.Background(), "s", "", []transcript.Message{{Role: "user", Content: strings.Repeat("Запрет. ", 12000)}})
	if err == nil || !strings.Contains(err.Error(), "coverage unchanged") {
		t.Fatalf("oversized input: %v", err)
	}
	value := strings.Repeat("Привет🦉", 50)
	if got := truncate(value, 12); !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Fatal("pipeline logging corrupted Unicode")
	}
}
