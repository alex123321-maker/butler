package telegramrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
)

func TestCommunicationRichDocumentVerificationBoundary(t *testing.T) {
	data := []byte("verified document\n")
	sum := sha256.Sum256(data)
	want := artifacts.Media{Name: "file.txt", MIME: "text/plain", SHA256: hex.EncodeToString(sum[:]), Data: data}
	for _, tc := range []struct {
		name, path, body string
		valid            bool
	}{
		{"valid", "documents/file.txt", string(data), true},
		{"changed", "documents/file.txt", "changed", false},
		{"truncated", "documents/file.txt", "", false},
		{"parent", "../file.txt", "", false},
		{"absolute", "/documents/file.txt", "", false},
		{"host", "https://example.com/file.txt", "", false},
		{"encoded", "documents/%2e%2e/file.txt", "", false},
		{"query", "documents/file.txt?token=x", "", false},
		{"backslash", "documents\\file.txt", "", false},
		{"double-separator", "documents//file.txt", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var downloads atomic.Int32
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/getFile") {
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_id": "fixture-file", "file_path": tc.path, "file_size": len(data)}})
					return
				}
				downloads.Add(1)
				_, _ = w.Write([]byte(tc.body))
			})
			err := c.VerifyDocument(context.Background(), "fixture-file", want)
			if (err == nil) != tc.valid {
				t.Fatal("document verification mismatch", err)
			}
			if tc.path != "documents/file.txt" && downloads.Load() != 0 {
				t.Fatal("untrusted path reached download")
			}
		})
	}
}

func TestCommunicationRichReadinessHasNoFallback(t *testing.T) {
	if len((Capabilities{}).Methods()) != 0 {
		t.Fatal("zero profile advertised tools")
	}
	p := planner()
	p.Profile.Document = false
	for _, m := range p.Profile.Methods() {
		if m == "communication.report" {
			t.Fatal("incomplete native profile advertised report")
		}
	}
	_, err := p.Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, communicationcontrol.Args{Title: "Report", Summary: "Summary", Blocks: []communicationcontrol.Block{{Type: "paragraph", Text: "Text"}}})
	if err == nil {
		t.Fatal("unsupported native report fell back")
	}
}
