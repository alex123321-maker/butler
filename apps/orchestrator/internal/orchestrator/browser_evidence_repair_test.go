package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/communicationcontrol"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/research"
	"github.com/butler/butler/internal/transport"
)

func TestBrowserEvidenceRepairRetainsFreshReadWithoutHTTP(t *testing.T) {
	now := time.Now().UTC()
	for _, mode := range []string{"live", "guarded_get", "foreign_run", "stale", "frozen", "local", "failed", "invented_quote"} {
		t.Run(mode, func(t *testing.T) {
			read := transcript.ToolCall{RunID: "r", ToolName: "browser.read", ArgsJSON: `{}`, Status: "completed", FinishedAt: &now,
				ResultJSON: `{"ok":true,"network_state":"read_only","final_url":"https://issuer.example/read","text":"The requested record reference is visible here."}`}
			switch mode {
			case "guarded_get":
				read.ToolName = "browser.follow_link"
				read.ResultJSON = strings.Replace(read.ResultJSON, "read_only", "frozen", 1)
			case "foreign_run":
				read.RunID = "other"
			case "stale":
				old := now.Add(-25 * time.Hour)
				read.FinishedAt = &old
			case "frozen":
				read.ResultJSON = strings.Replace(read.ResultJSON, "read_only", "frozen", 1)
			case "local":
				read.ResultJSON = strings.Replace(read.ResultJSON, `"ok":true`, `"ok":true,"local_preparation":true`, 1)
			case "failed":
				read.Status = "failed"
			}
			h := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{
				{RunID: "r", ToolName: "browser.prepare_fill", Status: "completed", FinishedAt: &now,
					ResultJSON: `{"ok":true,"local_preparation":true,"network_state":"frozen","final_url":"https://issuer.example/form","text":"The local draft contains the requested quantity."}`}, read,
			}}
			s := newFixtureService(nil, nil, nil, h, &mockProvider{}, Config{}, nil)
			args := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Text: "The requested record reference is visible here. https://issuer.example/read",
				Evidence: []research.Evidence{
					{URL: "https://issuer.example/form", Quote: "The local draft contains the requested quantity.", Role: "primary"},
					{URL: "https://issuer.example/read", Quote: "The requested record reference is visible here.", Role: "primary"},
				}}
			request := func() *transport.ToolCallRequest {
				raw, _ := json.Marshal(args)
				return &transport.ToolCallRequest{ToolName: "communication.message", ArgsJSON: string(raw)}
			}
			current := &sessionv1.RunRecord{RunId: "r", SessionKey: "chat"}
			feedback, err := s.researchFeedback(context.Background(), current, request())
			if err != nil || !strings.Contains(feedback, "Reuse supported current-run reads") || !strings.Contains(feedback, "unsupported claims/links") || strings.Contains(feedback, "Open the cited page with web.fetch") {
				t.Fatal("repair prescribed refetch or dropped the claim boundary")
			}
			// The model repairs only its draft. No new read, mutation or receipt is
			// supplied by this test or manufactured by the validation feedback.
			args.Evidence = args.Evidence[1:]
			if mode == "invented_quote" {
				args.Evidence[0].Quote = "This quote was never observed on any page."
			}
			feedback, err = s.researchFeedback(context.Background(), current, request())
			want := mode == "live" || mode == "guarded_get"
			if err != nil || (feedback == "") != want || len(h.toolCalls) != 2 {
				t.Fatal("repair changed evidence eligibility or required another read", mode)
			}
		})
	}
}
