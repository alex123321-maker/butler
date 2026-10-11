package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestClientCompressionAndReplayHeadersAreNotCopiedUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("compression negotiation or replay prevention failed")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/event-stream")
		z := gzip.NewWriter(w)
		_, _ = io.WriteString(z, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\""+marker+"\"}]},\"finishReason\":\"STOP\"}]}\n\n")
		_ = z.Close()
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	defer tr.CloseIdleConnections()
	g := gate{mode: "execute", token: "fixture", budget: &memoryAdmission{used: map[string]bool{}}, client: &http.Client{Transport: fakeRoundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
		return tr.RoundTrip(r)
	})}}
	r := httptest.NewRequest("POST", "https://aicode.googleapis.com/v1:streamGenerateContent?alt=sse", strings.NewReader(string(requestFixture())))
	r.Header.Set("User-Agent", "antigravity/1.3.3 butler/0.1")
	r.Header.Set("Authorization", "Bearer fixture")
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("Idempotency-Key", "fixture")
	r.Header.Set("X-Idempotency-Key", "fixture")
	w := httptest.NewRecorder()
	g.generate(w, r, requestFixture())
	if w.Code != 200 || len(g.results) != 1 || !g.results[0].MarkerMatch || !strings.HasPrefix(w.Body.String(), "data:") {
		t.Fatal("compressed SSE lost its decoded contents")
	}
}

func requestFixture() []byte {
	b, _ := json.Marshal(map[string]any{"aicode": map[string]string{"experience": flash}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": comparePrompt}}}}, "generationConfig": map[string]int{"maxOutputTokens": 8192}})
	return b
}

func TestAdmissionRejectsModelCreditsToolsHistoryDuplicateAndBounds(t *testing.T) {
	base := string(requestFixture())
	for name, input := range map[string]string{
		"wrong_model":  strings.Replace(base, flash, "chat-gemini-3.1-pro-low", 1),
		"credits":      strings.Replace(base, `"aicode"`, `"enabledCreditTypes":["G1"],"aicode"`, 1),
		"tools":        strings.Replace(base, `"aicode"`, `"tools":[{"codeExecution":{}}],"aicode"`, 1),
		"duplicate":    strings.Replace(base, `"aicode"`, `"contents":[],"aicode"`, 1),
		"case_alias":   strings.Replace(base, `"aicode"`, `"Contents":[],"aicode"`, 1),
		"other_prompt": strings.Replace(base, marker, "private-fixture", 1),
		"trailing":     base + ` {}`,
		"oversize":     strings.Repeat(" ", requestLimit) + base,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := normalize("aicode.googleapis.com", "/v1:streamGenerateContent", []byte(input)); err == nil {
				t.Fatal("unsafe write admitted")
			}
		})
	}
	if _, _, err := normalize("aicode.googleapis.com.evil.invalid", "/v1:streamGenerateContent", requestFixture()); err == nil {
		t.Fatal("arbitrary host admitted")
	}
}

func TestBothEnvelopesReceiveExplicitBoundWithoutIdentityExport(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		body := requestFixture()
		host, path := "aicode.googleapis.com", "/v1:streamGenerateContent"
		if cloud {
			var value map[string]any
			_ = json.Unmarshal(body, &value)
			delete(value, "aicode")
			b, _ := json.Marshal(map[string]any{"model": cliFlash, "project": "private-profile-fixture", "requestId": "private-profile-fixture", "request": value})
			body = b
			host, path = "daily-cloudcode-pa.googleapis.com", "/v1internal:streamGenerateContent"
		}
		b, p, err := normalize(host, path, body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"maxOutputTokens":512`) {
			t.Fatal("output bound absent")
		}
		safe, _ := json.Marshal(p)
		if strings.Contains(string(safe), "private-profile-fixture") || strings.Contains(string(safe), comparePrompt) {
			t.Fatal("identity or body exported")
		}
	}
}

type memoryAdmission struct {
	mu    sync.Mutex
	used  map[string]bool
	spent int
}

func (m *memoryAdmission) Reserve(_ context.Context, c string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.used[c] || m.spent >= 2 {
		return 0, denied
	}
	m.used[c] = true
	m.spent++
	return 18 + m.spent, nil
}
func (m *memoryAdmission) Observe(context.Context, int, observation) error { return nil }

type fakeRoundTrip func(*http.Request) (*http.Response, error)

func (f fakeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInspectNeverWritesAndExecuteDeniesRetryBeforeNetwork(t *testing.T) {
	writes := 0
	client := &http.Client{Transport: fakeRoundTrip(func(r *http.Request) (*http.Response, error) {
		writes++
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"maxOutputTokens":512`) {
			t.Fatal("unbounded upstream")
		}
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"private-fixture","details":[{"reason":"HIGH_DEMAND","metadata":{"email":"private-fixture"}}]}}`))}, nil
	})}
	g := gate{mode: "inspect", client: client, token: "fixture", budget: &memoryAdmission{used: map[string]bool{}}, onGeneration: make(chan struct{}, 1)}
	call := func() {
		r := httptest.NewRequest("POST", "https://aicode.googleapis.com/v1:streamGenerateContent?alt=sse", strings.NewReader(string(requestFixture())))
		r.Header.Set("User-Agent", "antigravity/1.3.3 butler/0.1")
		r.Header.Set("Authorization", "Bearer fixture")
		g.generate(httptest.NewRecorder(), r, requestFixture())
	}
	call()
	if writes != 0 {
		t.Fatal("inspect performed inference")
	}
	g.mode = "execute"
	call()
	call()
	if writes != 1 {
		t.Fatal("retry reached upstream")
	}
	if len(g.results) != 1 || g.results[0].Reason != "HIGH_DEMAND" || g.results[0].InputTokens != nil {
		t.Fatal("reason or unknown usage lost")
	}
	b, _ := json.Marshal(g.results)
	if strings.Contains(string(b), "private-fixture") {
		t.Fatal("private error released")
	}
}

func TestErrorAndSSEResponseKeepUnknownUsageAndSeparateThoughts(t *testing.T) {
	o := observation{Reason: "UNKNOWN", APIStatus: "UNKNOWN"}
	projectResponse([]byte(`{"error":{"details":[{"reason":"OTHER","message":"private-fixture"}]}}`), &o)
	if o.Reason != "UNKNOWN" || o.InputTokens != nil || o.OutputTokens != nil {
		t.Fatal("invented capacity or zero usage")
	}
	frame := `{"response":{"candidates":[{"content":{"parts":[{"text":"private-thinking","thought":true},{"text":"` + marker + `"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"thoughtsTokenCount":3,"totalTokenCount":9}}}`
	projectResponse([]byte("data: "+frame+"\n\n"), &o)
	if !o.MarkerMatch || o.Finish != "STOP" || *o.InputTokens != 4 || *o.OutputTokens != 2 || *o.ThoughtTokens != 3 || *o.TotalTokens != 9 {
		t.Fatal("SSE/usage projection failed")
	}
	b, _ := json.Marshal(o)
	if strings.Contains(string(b), "private-thinking") {
		t.Fatal("thought exported")
	}
	bad := observation{}
	projectResponse([]byte("data: "+frame+"\n\ndata: {broken\n\n"), &bad)
	if !bad.MalformedResponse {
		t.Fatal("valid prefix concealed malformed trailing SSE")
	}
}

func TestMetadataAndControlHostsDenyWritesAndArbitraryDestinations(t *testing.T) {
	for _, path := range []string{"/v1internal:onboardUser", "/v1internal:purchaseCredits", "/token", "/v1:generateContent"} {
		if metadataAllowed("cloudcode-pa.googleapis.com", path, "POST") {
			t.Fatal("non-metadata admitted")
		}
	}
	if proxyHost("evil.invalid") || metadataAllowed("evil.invalid", "/v1:fetchConfig", "GET") {
		t.Fatal("host bypass")
	}
	if !avatarHost("lh3.googleusercontent.com") || avatarHost("lh3.googleusercontent.com.evil.invalid") || metadataAllowed("lh3.googleusercontent.com", "/arbitrary", "GET") {
		t.Fatal("avatar scope accepts arbitrary metadata")
	}
}

func TestCLIWrappedPromptDeclarationAndProposalBlocking(t *testing.T) {
	var request map[string]any
	_ = json.Unmarshal(requestFixture(), &request)
	delete(request, "aicode")
	request["contents"] = []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": "<fresh diagnostic reminder>\n" + comparePrompt}}}}
	request["tools"] = []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "fixture", "parameters": map[string]string{"type": "object"}}}}}
	body, _ := json.Marshal(map[string]any{"model": cliFlash, "request": request})
	if _, p, err := normalize("daily-cloudcode-pa.googleapis.com", "/v1internal:streamGenerateContent", body); err != nil || p["tools_absent"] != false || p["tool_execution_disabled"] != true {
		t.Fatal("installed CLI shape not recognized")
	}
	g := gate{mode: "execute", token: "fixture-access", budget: &memoryAdmission{used: map[string]bool{}}, client: &http.Client{Transport: fakeRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"fixture\",\"args\":{\"private\":\"fixture-secret\"}}}]},\"finishReason\":\"STOP\"}]}}\n\n"))}, nil
	})}}
	r := httptest.NewRequest("POST", "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse", strings.NewReader(string(body)))
	r.Header.Set("User-Agent", "antigravity/cli/1.3.3 fixture")
	r.Header.Set("Authorization", "Bearer fixture-access")
	w := httptest.NewRecorder()
	g.generate(w, r, body)
	if w.Code != 502 || strings.Contains(w.Body.String(), "functionCall") || len(g.results) != 1 || !g.results[0].ToolProposal || g.results[0].Outcome != "TOOL_PROPOSAL_BLOCKED" {
		t.Fatal("proposal reached CLI executor")
	}
}

func TestNative8192PreservesExactBodyAndRejectsCapChange(t *testing.T) {
	b := requestFixture()
	got, p, err := normalizeBound("aicode.googleapis.com", "/v1:streamGenerateContent", b, 8192)
	if err != nil || string(got) != string(b) || p["original_bytes_preserved"] != true {
		t.Fatal("production request bytes changed")
	}
	changed := strings.Replace(string(b), "8192", "512", 1)
	if _, _, err := normalizeBound("aicode.googleapis.com", "/v1:streamGenerateContent", []byte(changed), 8192); err == nil {
		t.Fatal("unproved output cap admitted")
	}
	if _, _, err := normalizeBound("aicode.googleapis.com", "/v1:streamGenerateContent", b, 16384); err == nil {
		t.Fatal("expanded profile admitted")
	}
}

func TestNativeOnlyDiagnosticDoesNotClaimCachedCLIOutput(t *testing.T) {
	d := cliDiagnosticsForMode("native8192")
	if d["available"] != false || d["cli_launched"] != false || d["cached_output_ignored"] != true {
		t.Fatal("previous CLI result attributed to native-only window")
	}
}
