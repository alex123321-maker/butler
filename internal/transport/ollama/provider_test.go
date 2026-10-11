package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

const tagsJSON = `{"models":[{"name":"qwen3:0.6b","model":"qwen3:0.6b","digest":"` + ModelDigest + `"}]}`
const finalJSON = `{"model":"qwen3:0.6b","message":{"role":"assistant","content":"Готово"},"done":true,"done_reason":"stop","prompt_eval_count":12,"eval_count":3}`

func fixture(t *testing.T, tags string, handler http.HandlerFunc) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, tags)
			return
		}
		if r.URL.Path != "/api/chat" {
			t.Error("unexpected endpoint")
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	p, err := NewProvider(Config{Endpoint: server.URL, Digest: ModelDigest, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func request(run string) transport.StartRunRequest {
	return transport.StartRunRequest{Context: transport.TransportRunContext{RunID: run, SessionKey: "owner:" + run, ProviderName: Name, ModelName: Model}, StreamingEnabled: true, InputItems: []transport.InputItem{content.CurrentOwnerMessage("Проверь источник", "input:"+run)}}
}
func drain(t *testing.T, p *Provider, r transport.StartRunRequest) []transport.TransportEvent {
	t.Helper()
	s, err := p.StartRun(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	var events []transport.TransportEvent
	for event := range s {
		events = append(events, event)
	}
	return events
}
func finalOf(events []transport.TransportEvent) *transport.AssistantFinal {
	for _, e := range events {
		if e.AssistantFinal != nil {
			return e.AssistantFinal
		}
	}
	return nil
}

func TestPinnedRequestAndUsage(t *testing.T) {
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != Model || !body.Stream || body.Think || body.Options.NumCtx != ContextTokens || body.Options.NumPredict != OutputTokens || body.Options.NumThread != 2 || body.Options.Temperature != 0 || body.KeepAlive != "5m" {
			t.Error("profile drift")
		}
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "Проверь источник" {
			t.Error("context drift")
		}
		fmt.Fprintln(w, finalJSON)
	})
	events := drain(t, p, request("one"))
	f := finalOf(events)
	if f == nil || f.Content != "Готово" || f.Usage == nil || f.Usage.InputTokens != 12 || f.Usage.OutputTokens != 3 || f.Usage.TotalTokens != 15 || events[len(events)-1].EventType != transport.EventTypeRunCompleted {
		t.Fatal("unverified final or usage")
	}
	cap, _ := p.Capabilities(context.Background(), request("one").Context)
	if cap.SupportsToolCalls || cap.SupportsBatchToolCalls || cap.SupportsStatefulSessions {
		t.Fatal("capability escalated")
	}
}

func TestUsageAbsenceDoesNotBecomeZero(t *testing.T) {
	for _, suffix := range []string{"", `,"prompt_eval_count":null,"eval_count":null`, `,"prompt_eval_count":12`} {
		t.Run(suffix, func(t *testing.T) {
			p := fixture(t, tagsJSON, func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"model":"qwen3:0.6b","message":{"role":"assistant","content":"OK"},"done":true,"done_reason":"stop"%s}`+"\n", suffix)
			})
			f := finalOf(drain(t, p, request("missing")))
			if f == nil || f.Usage != nil {
				t.Fatal("missing usage presented as zero")
			}
		})
	}
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, strings.ReplaceAll(strings.ReplaceAll(finalJSON, `:12`, `:0`), `:3`, `:0`))
	})
	if f := finalOf(drain(t, p, request("explicit-zero"))); f == nil || f.Usage == nil || f.Usage.TotalTokens != 0 {
		t.Fatal("explicit zero lost")
	}
}

func TestInvalidStreamsRefuseFinalAndMaskFailures(t *testing.T) {
	cases := map[string]string{
		"incomplete":        `{"model":"qwen3:0.6b","message":{"role":"assistant","content":"partial"},"done":false}`,
		"duplicate":         strings.Replace(finalJSON, `"eval_count":3`, `"eval_count":3,"eval_count":0`, 1),
		"escaped-duplicate": strings.Replace(finalJSON, `"eval_count":3`, `"eval_count":3,"eval\u005fcount":0`, 1),
		"negative":          strings.Replace(finalJSON, `:12`, `:-1`, 1),
		"partial-negative":  strings.Replace(strings.Replace(finalJSON, `:12`, `:-1`, 1), `"eval_count":3`, `"eval_count":null`, 1),
		"over-input":        strings.Replace(finalJSON, `:12`, `:4097`, 1),
		"over-output":       strings.Replace(finalJSON, `:3`, `:257`, 1),
		"wrong-model":       strings.Replace(finalJSON, Model, "other", 1),
		"raw-error":         `{"error":"SECRET_DIAGNOSTIC_MARKER password=fixture-private"}`,
		"tool-call":         strings.Replace(finalJSON, `"content":"Готово"`, `"content":"Готово","tool_calls":[{}]`, 1),
		"thinking":          strings.Replace(finalJSON, `"content":"Готово"`, `"content":"Готово","thinking":"private thought"`, 1),
		"multiple-finals":   finalJSON + "\n" + finalJSON,
		"unknown-stop":      strings.Replace(finalJSON, `"stop"`, `"SECRET_DIAGNOSTIC_MARKER"`, 1),
		"oversized":         strings.Repeat("a", 65537),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			p := fixture(t, tagsJSON, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, raw) })
			events := drain(t, p, request(name))
			if finalOf(events) != nil || len(events) == 0 || events[len(events)-1].TransportError == nil {
				t.Fatal("invalid stream passed")
			}
			last := events[len(events)-1]
			if strings.Contains(last.PayloadJSON, "SECRET_DIAGNOSTIC_MARKER") || strings.Contains(last.PayloadJSON, "fixture-private") || strings.Contains(last.PayloadJSON, "private thought") {
				t.Fatal("raw failure leaked")
			}
		})
	}
}

func TestRedirectNeverReachesAnotherEndpoint(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprintln(w, tagsJSON) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	p, err := NewProvider(Config{Endpoint: origin.URL, Digest: ModelDigest, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, p, request("redirect"))
	if calls.Load() != 0 || finalOf(events) != nil || events[len(events)-1].TransportError == nil {
		t.Fatal("redirect escaped private boundary")
	}
}

func TestPinRefusalBeforeInference(t *testing.T) {
	for _, tags := range []string{`{"models":[]}`, strings.Replace(tagsJSON, ModelDigest, strings.Repeat("0", 64), 1), strings.Replace(tagsJSON, `"digest":`, `"digest":"bad","digest":`, 1)} {
		var calls atomic.Int32
		p := fixture(t, tags, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
		events := drain(t, p, request("pin"))
		if calls.Load() != 0 || finalOf(events) != nil || events[len(events)-1].TransportError == nil {
			t.Fatal("unpinned inference")
		}
	}
}

func TestPreHTTPBoundsAndAuthority(t *testing.T) {
	var calls atomic.Int32
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); fmt.Fprintln(w, finalJSON) })
	for _, mutate := range []func(*transport.StartRunRequest){
		func(r *transport.StartRunRequest) { r.ToolDefinitions = []transport.ToolDefinition{{Name: "exec"}} },
		func(r *transport.StartRunRequest) {
			r.TransportOptions = map[string]any{"endpoint": "https://external.example"}
		},
		func(r *transport.StartRunRequest) { r.Context.ModelName = "other" },
		func(r *transport.StartRunRequest) { r.Context.ProviderSessionRef = &transport.ProviderSessionRef{} },
		func(r *transport.StartRunRequest) {
			r.InputItems = []transport.InputItem{content.Data("synthetic secret", content.Origin{Kind: content.Document, Sensitivity: content.Secret})}
		},
		func(r *transport.StartRunRequest) { r.InputItems[0].Content = strings.Repeat("я", 2048) },
	} {
		r := request("refuse")
		mutate(&r)
		if _, err := p.StartRun(context.Background(), r); err == nil {
			t.Fatal("unsafe request admitted")
		}
	}
	if _, err := p.StartRun(transport.WithInputByteLimit(context.Background(), 1), request("bytes")); err == nil {
		t.Fatal("serialized cap bypassed")
	}
	if calls.Load() != 0 {
		t.Fatal("inference before refusal")
	}
}

func TestLegacyInstructionsRemainData(t *testing.T) {
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || !strings.Contains(body.Messages[0].Content, `"original_role":"system"`) {
			t.Error("legacy instruction elevated")
		}
		fmt.Fprintln(w, finalJSON)
	})
	r := request("legacy")
	r.InputItems = []transport.InputItem{{Role: "system", Content: "untrusted external instruction"}}
	if finalOf(drain(t, p, r)) == nil {
		t.Fatal("data normalization failed")
	}
}

func TestSharedProviderSerializesContextsAndCancelsQueuedRun(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			fmt.Fprintln(w, finalJSON)
		case <-r.Context().Done():
		}
	})
	s1, err := p.StartRun(context.Background(), request("first"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	s2, err := p.StartRun(context.Background(), request("second"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "second"}); err != nil {
		t.Fatal(err)
	}
	for e := range s2 {
		if e.AssistantFinal != nil {
			t.Fatal("cancelled run finalized")
		}
	}
	close(release)
	for range s1 {
	}
	if calls.Load() != 1 {
		t.Fatal("queued context reached inference")
	}
	drain(t, p, request("third"))
	if calls.Load() != 2 {
		t.Fatal("shared service did not accept next context")
	}
}

func TestUnsafeEndpointAndUnsupportedContinuation(t *testing.T) {
	for _, endpoint := range []string{"https://remote.example:11434", "http://remote.example:11434", "http://localhost:11434", "http://127.0.0.1:11434/", "http://u:p@127.0.0.1:11434", "http://127.0.0.1:11434?x=1", "http://butler-generative:80"} {
		if _, err := NewProvider(Config{Endpoint: endpoint, Digest: ModelDigest, Timeout: time.Second}); err == nil {
			t.Fatal("unsafe endpoint admitted")
		}
	}
	p := fixture(t, tagsJSON, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, finalJSON) })
	if _, err := p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "one"}); err == nil {
		t.Fatal("invented resume")
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "one"}); err == nil {
		t.Fatal("invented tool execution")
	}
}
