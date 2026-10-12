package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/butler/butler/internal/transport"
)

const privateFinalMaximumBytes = 8192
const privateFinalMaximumDepth = 64

type privateFinalMetadata struct {
	Bytes       int    `json:"visible_final_bytes"`
	SHA256      string `json:"visible_final_sha256"`
	Reason      string `json:"encoding_reason"`
	RawRetained bool   `json:"private_raw_retained"`
}

// Classifies encoding only, never the schema, source witness, golden answer or
// model reasoning. Every reason is code-owned and contains no observed value.
func classifyPrivateFinal(raw []byte) string {
	if len(raw) > privateFinalMaximumBytes {
		return "visible_bound"
	}
	if !utf8.Valid(raw) {
		return "invalid_utf8"
	}
	if !json.Valid(raw) {
		return "json_syntax"
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "non_object"
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value func(int) string
	value = func(depth int) string {
		token, err := decoder.Token()
		if err != nil {
			return "json_syntax"
		}
		delimiter, nested := token.(json.Delim)
		if !nested {
			return "json_object"
		}
		if depth > privateFinalMaximumDepth {
			return "depth_bound"
		}
		if delimiter == '{' {
			keys := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok {
					return "json_syntax"
				}
				if keys[name] {
					return "duplicate_key"
				}
				keys[name] = true
				if reason := value(depth + 1); reason != "json_object" {
					return reason
				}
			}
		} else if delimiter == '[' {
			for decoder.More() {
				if reason := value(depth + 1); reason != "json_object" {
					return reason
				}
			}
		} else {
			return "json_syntax"
		}
		if _, err := decoder.Token(); err != nil {
			return "json_syntax"
		}
		return "json_object"
	}
	return value(1)
}

// The private directory and ordinal come only from the owning runner. Raw
// bytes remain a separate0600 file; this metadata is safe to serialize publicly.
func capturePrivateFinal(private string, ordinal int, content string) (privateFinalMetadata, error) {
	raw := []byte(content)
	hash := sha256.Sum256(raw)
	m := privateFinalMetadata{Bytes: len(raw), SHA256: hex.EncodeToString(hash[:]), Reason: classifyPrivateFinal(raw)}
	if len(raw) > privateFinalMaximumBytes {
		return m, nil
	}
	stat, err := os.Lstat(private)
	if ordinal <= 0 || !filepath.IsAbs(private) || err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return m, errNativeFixtureDenied
	}
	file, err := os.OpenFile(filepath.Join(private, fmt.Sprintf("final-%03d.raw", ordinal)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return m, errNativeFixtureDenied
	}
	written, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || written != len(raw) || syncErr != nil || closeErr != nil {
		return m, errNativeFixtureDenied
	}
	m.RawRetained = true
	return m, nil
}

// Future diagnostic-only observer. It neither admits a provider call nor
// repairs/rewraps its output. Only a matching filtered AssistantFinal is captured;
// deltas, thoughts, signatures and provider transport frames are never stored.
type privateFinalCaptureProvider struct {
	transport.ModelProvider
	private       string
	ordinal       int
	mu            sync.Mutex
	metadata      *privateFinalMetadata
	captureFailed bool
}

func (p *privateFinalCaptureProvider) finalMetadata() *privateFinalMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.metadata == nil {
		return nil
	}
	copy := *p.metadata
	return &copy
}

func (p *privateFinalCaptureProvider) captureFailure() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.captureFailed
}

func (*privateFinalCaptureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errNativeFixtureDenied
}
func (*privateFinalCaptureProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errNativeFixtureDenied
}

func (p *privateFinalCaptureProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.StartRun(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-stream:
				if !open {
					return
				}
				if event.RunID == req.Context.RunID && event.EventType == transport.EventTypeAssistantFinal && event.AssistantFinal != nil {
					p.mu.Lock()
					seen := p.metadata != nil
					p.mu.Unlock()
					if seen {
						p.mu.Lock()
						p.captureFailed = true
						p.mu.Unlock()
					} else {
						m, captureErr := capturePrivateFinal(p.private, p.ordinal, event.AssistantFinal.Content)
						p.mu.Lock()
						p.metadata = &m
						p.captureFailed = captureErr != nil
						p.mu.Unlock()
					}
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func TestScenarioPrivateFinalCaptureEncodingBoundsAndMetadataPrivacy(t *testing.T) {
	for _, tc := range []struct{ name, text, reason string }{
		{"object", `{"label":"PRIVATE_MODEL_BODY"}`, "json_object"},
		{"syntax", strings.Repeat("PRIVATE_MODEL_BODY", 250), "json_syntax"},
		{"string", `"PRIVATE_MODEL_BODY"`, "non_object"},
		{"array", `[]`, "non_object"},
		{"null", `null`, "non_object"},
		{"duplicate", `{"a":1,"\u0061":2}`, "duplicate_key"},
		{"nested_duplicate", `{"x":[{"a":1,"a":2}]}`, "duplicate_key"},
		{"depth", strings.Repeat(`{"x":`, 65) + `0` + strings.Repeat(`}`, 65), "depth_bound"},
		{"depth64", strings.Repeat(`{"x":`, 64) + `0` + strings.Repeat(`}`, 64), "json_object"},
		{"utf8", string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}), "invalid_utf8"},
		{"visible", strings.Repeat("x", 8193), "visible_bound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private := t.TempDir()
			m, err := capturePrivateFinal(private, 267, tc.text)
			if err != nil || m.Reason != tc.reason || m.Bytes != len(tc.text) || m.RawRetained != (len(tc.text) <= 8192) {
				t.Fatal("fixed encoding metadata or retention bound differs")
			}
			body, err := json.Marshal(m)
			if err != nil || bytes.Contains(body, []byte("PRIVATE_MODEL_BODY")) || bytes.Contains(body, []byte(private)) {
				t.Fatal("visible model content or private path escaped metadata")
			}
			path := filepath.Join(private, "final-267.raw")
			if m.RawRetained {
				raw, err := os.ReadFile(path)
				stat, statErr := os.Stat(path)
				if err != nil || statErr != nil || stat.Mode().Perm() != 0600 || !bytes.Equal(raw, []byte(tc.text)) {
					t.Fatal("private bytes were repaired, truncated or made public")
				}
				if _, err := capturePrivateFinal(private, 267, "replacement"); err == nil {
					t.Fatal("immutable private observation overwritten")
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("oversized model output retained")
			}
		})
	}
}
