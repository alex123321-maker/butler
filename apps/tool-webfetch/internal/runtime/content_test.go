package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func executeContentFixture(t *testing.T, tool string, args map[string]any, handler http.HandlerFunc) (*toolbrokerv1.ToolResult, map[string]any) {
	t.Helper()
	site := httptest.NewServer(handler)
	t.Cleanup(site.Close)
	s := NewServer(NewProviderChainWithPolicy(egress.Policy{Resolver: egresstest.Resolver{}}, NewPlainHTTPProvider(egresstest.Client(site), true)), nil)
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	response, err := s.Execute(context.Background(), &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "content", RunId: "run", ToolName: tool, ArgsJson: string(raw)}})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if response.GetResult().GetResultJson() != "" {
		if err := json.Unmarshal([]byte(response.GetResult().GetResultJson()), &payload); err != nil {
			t.Fatal(err)
		}
	}
	return response.GetResult(), payload
}

func TestReadableTextExcludesScriptAndStyle(t *testing.T) {
	for _, tool := range []string{"web.fetch", "web.extract"} {
		t.Run(tool, func(t *testing.T) {
			r, p := executeContentFixture(t, tool, map[string]any{"url": egresstest.Origin}, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, `<html><head><style>STYLE_NOISE</style></head><body><main>Статья <b>about Go</b></main><script>SCRIPT_NOISE</script></body></html>`)
			})
			if r.GetStatus() != "completed" || p["content_text"] != "Статья about Go" {
				t.Fatalf("readable text changed or contains script/style: %s %v", r.GetStatus(), p)
			}
		})
	}
}

func TestExtractRefusesUnsuccessfulHTTPStatus(t *testing.T) {
	for _, status := range []int{301, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			r, _ := executeContentFixture(t, "web.extract", map[string]any{"url": egresstest.Origin, "text": "caller text cannot mask an HTTP failure"}, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(status)
				fmt.Fprint(w, "private error document")
			})
			var details map[string]any
			if err := json.Unmarshal([]byte(r.GetError().GetDetailsJson()), &details); err != nil {
				t.Fatal(err)
			}
			if r.GetStatus() != "failed" || details["status_code"] != float64(status) || strings.Contains(r.String(), "private error document") {
				t.Fatalf("HTTP failure lost or body leaked: %v", r)
			}
		})
	}
}

func TestTextLimitReportsTruncationAndKeepsUTF8(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		truncated bool
	}{
		{"exact", strings.Repeat("x", maxContentBytes), false},
		{"oversized", strings.Repeat("x", maxContentBytes+1), true},
		{"split rune", strings.Repeat("x", maxContentBytes-1) + "Ж", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, tool := range []string{"web.fetch", "web.extract"} {
				r, p := executeContentFixture(t, tool, map[string]any{"url": egresstest.Origin}, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/plain")
					fmt.Fprint(w, tc.body)
				})
				meta, ok := p["metadata"].(map[string]any)
				text, _ := p["content_text"].(string)
				if r.GetStatus() != "completed" || !ok || meta["truncated"] != tc.truncated || len(text) > maxContentBytes || !utf8.ValidString(text) {
					t.Fatalf("%s: invalid completeness/UTF8: status=%s metadata=%v bytes=%d", tool, r.GetStatus(), meta, len(text))
				}
			}
		})
	}
}

func TestBinaryContentNeverBecomesReadableText(t *testing.T) {
	for _, tc := range []struct{ name, mime, body string }{
		{"PDF", "application/pdf", "%PDF-1.7\n\x00\xff"},
		{"sniffed PDF", "", "%PDF-1.7\n\x00\xff"},
		{"image", "image/png", "PNG"},
		{"octet stream", "application/octet-stream", "binary"},
		{"invalid UTF8", "text/plain", "text\xff"},
		{"invalid UTF8 at truncation boundary", "text/plain", strings.Repeat("x", maxContentBytes-1) + "\xffmore"},
		{"NUL", "text/plain", "text\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := executeContentFixture(t, "web.fetch", map[string]any{"url": egresstest.Origin}, func(w http.ResponseWriter, _ *http.Request) {
				// Setting an empty header prevents net/http from sniffing for us.
				w.Header()["Content-Type"] = []string{tc.mime}
				fmt.Fprint(w, tc.body)
			})
			if r.GetStatus() != "failed" || !strings.Contains(r.GetError().GetDetailsJson(), "unsupported_content") || r.GetResultJson() != "" {
				t.Fatalf("unsupported content published: %v", r)
			}
		})
	}
}

func TestEmptySuccessfulExtractPreservesProvenance(t *testing.T) {
	r, p := executeContentFixture(t, "web.extract", map[string]any{"url": egresstest.Origin}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(204)
	})
	if r.GetStatus() != "completed" || p["status_code"] != float64(204) || p["content_text"] != "" || p["final_url"] != egresstest.Origin {
		t.Fatalf("empty document confused with missing input: %s %v", r.GetStatus(), p)
	}
}

func TestExpandedOutputIsRefusedBeforeGRPCResponseLimit(t *testing.T) {
	r, _ := executeContentFixture(t, "web.fetch", map[string]any{"url": egresstest.Origin}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("\"", maxContentBytes))
	})
	if r.GetStatus() != "failed" || !strings.Contains(r.GetError().GetMessage(), "output exceeds") {
		t.Fatalf("oversized serialized output not explicitly refused: %s", r.GetStatus())
	}
}
