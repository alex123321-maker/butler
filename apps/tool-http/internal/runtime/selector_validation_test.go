package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestUnsupportedSelectorRefusedBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer site.Close()
	s := NewServer(egresstest.Client(site), nil)
	for _, selector := range []string{"article h1", "div>p", "a[href]", "h1,h2", ".first.second", "p#id.class", "p:", ".", "#", "p.", "p#", "*", "#a#b"} {
		t.Run(selector, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]string{"url": egresstest.Origin, "selector": selector})
			r, err := s.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.parse_html", ArgsJson: string(raw)}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"}})
			if err != nil || r.GetResult().GetStatus() != "failed" || requests.Load() != 0 {
				t.Fatalf("unsupported selector performed a request or returned success: %v %v", r, err)
			}
		})
	}
}

func TestNoSelectorMatchIsEmptyArray(t *testing.T) {
	s := NewServer(nil, nil)
	for _, selector := range []string{"h2", ".absent", "#absent", "h1.absent", "h1#absent", ".класс"} {
		t.Run(selector, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]string{"content": "<h1>Present</h1>", "selector": selector})
			r, err := s.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: "http.parse_html", ArgsJson: string(raw)}, Contract: &toolbrokerv1.ToolContract{ToolName: "http.parse_html"}})
			if err != nil || r.GetResult().GetStatus() != "completed" {
				t.Fatalf("valid selector rejected: %v %v", r, err)
			}
			var payload struct {
				Matches []string `json:"matches"`
				Count   int      `json:"count"`
			}
			if err := json.Unmarshal([]byte(r.GetResult().GetResultJson()), &payload); err != nil || payload.Matches == nil || payload.Count != 0 {
				t.Fatal(fmt.Sprintf("no-match result is not an empty array: %s", r.GetResult().GetResultJson()))
			}
		})
	}
}
