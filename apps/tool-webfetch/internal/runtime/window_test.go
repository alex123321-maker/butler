package runtime

import (
	"context"
	"encoding/json"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"strings"
	"testing"
)

func TestTextWindowArchiveBoundContinuationAndVersion(t *testing.T) {
	body := strings.Repeat("Актуальный выпуск. ", 2000)
	r, err := windowFetch(FetchResult{ContentText: body, ContentHTML: strings.Repeat("<p>old</p>", 2000)}, 0, 0, "")
	if err != nil || len([]rune(r.ContentText)) != defaultTextCharacters || r.Window.NextOffset == nil || r.Metadata["truncated"] != true || r.Metadata["html_truncated"] != true {
		t.Fatal(r.Window, err)
	}
	second, err := windowFetch(FetchResult{ContentText: body}, *r.Window.NextOffset, 1000, r.Window.TextHash)
	if err != nil || second.Window.Offset != defaultTextCharacters || len([]rune(second.ContentText)) != 1000 {
		t.Fatal(second.Window, err)
	}
	if _, err = windowFetch(FetchResult{ContentText: body + "changed"}, *r.Window.NextOffset, 1000, r.Window.TextHash); err == nil {
		t.Fatal("stitched different versions")
	}
	for _, tc := range []struct {
		offset, limit int
		hash          string
	}{{-1, 0, ""}, {0, 1, ""}, {0, 24001, ""}, {len([]rune(body)) + 1, 0, ""}, {0, 0, "not-a-hash"}} {
		if _, err = windowFetch(FetchResult{ContentText: body}, tc.offset, tc.limit, tc.hash); err == nil {
			t.Fatal("invalid window accepted", tc)
		}
	}
}

func TestTextWindowRuntimeReadsAndBatchCannotFloodContext(t *testing.T) {
	body := strings.Repeat("Release history. ", 10000)
	s := NewServer(NewProviderChainWithPolicy(egress.Policy{Resolver: egresstest.Resolver{}}, fakeProvider{name: "fixture", result: FetchResult{FinalURL: "https://example.com/", StatusCode: 200, MIMEType: "text/plain", ContentText: body}}), nil)
	for _, tc := range []struct{ name, args string }{{"web.fetch", `{"url":"https://example.com/"}`}, {"web.extract", `{"url":"https://example.com/"}`}, {"web.fetch_batch", `{"urls":["https://example.com/a","https://example.com/b","https://example.com/c"]}`}} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := s.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: tc.name, RunId: "r", ToolName: tc.name, ArgsJson: tc.args}})
			if err != nil || r.Result.Status != "completed" {
				t.Fatal(r, err)
			}
			var out struct {
				Content string        `json:"content_text"`
				Window  *TextWindow   `json:"content_window"`
				Items   []FetchResult `json:"items"`
			}
			if json.Unmarshal([]byte(r.Result.ResultJson), &out) != nil {
				t.Fatal("invalid runtime response")
			}
			if tc.name == "web.fetch_batch" {
				total := 0
				for _, item := range out.Items {
					total += len([]rune(item.ContentText))
					if item.Window == nil || item.Window.NextOffset == nil {
						t.Fatal("batch hid truncation")
					}
				}
				if total > maxTextCharacters {
					t.Fatal("batch multiplied page budget", total)
				}
			} else if len([]rune(out.Content)) != defaultTextCharacters || out.Window == nil || out.Window.NextOffset == nil {
				t.Fatal("raw archive reached model", len(out.Content), out.Window)
			}
		})
	}
}

func TestTextWindowPreservesSmallPageAndUpstreamTruncation(t *testing.T) {
	r, err := windowFetch(FetchResult{ContentText: "The event has moved to Monday.", Metadata: map[string]any{"truncated": true, "upstream": "bounded"}}, 0, 0, "")
	if err != nil || r.Window.NextOffset != nil || r.Window.Available != len([]rune(r.ContentText)) || r.Metadata["truncated"] != true {
		t.Fatal(r, err)
	}
}
