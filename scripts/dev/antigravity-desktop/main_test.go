package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSyntheticProjectionNeverExportsIdentityTokensOrPrompt(t *testing.T) {
	r := httptest.NewRequest("POST", "https://aicode.googleapis.com/v1:streamGenerateContent?alt=sse", nil)
	r.Header.Set("Authorization", "Bearer synthetic-access")
	body := []byte(`{"aicode":{"experience":"gemini-3.8-flash-tiered"},"sessionId":"PRIVATE-SESSION","private-key":"PRIVATE-FIELD","contents":[{"role":"user","parts":[{"text":"PRIVATE-TEXT"}]}],"generationConfig":{"maxOutputTokens":8192,"thinkingConfig":{"thinkingLevel":"HIGH"}}}`)
	b, _ := json.Marshal(projection(r, body))
	for _, private := range []string{"PRIVATE", "synthetic-access", "Bearer", "private-key"} {
		if strings.Contains(string(b), private) {
			t.Fatal("private material exported")
		}
	}
	v := projection(r, body)
	if v["model"] != "gemini-3.8-flash-tiered" || v["thinking_level"] != "HIGH" || v["unknown_top_key_count"] != 1 {
		t.Fatal("safe comparison fields lost")
	}
}

type desktopRoundTrip func(*http.Request) (*http.Response, error)

func (f desktopRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMetadataAvatarRequiresRegisteredURLAndDropsCredentials(t *testing.T) {
	calls := 0
	g := &metadataGateway{deadline: time.Now().Add(time.Minute), token: "PRIVATE-TOKEN", avatarURL: "https://lh3.googleusercontent.com/PRIVATE-AVATAR", client: &http.Client{Transport: desktopRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("credential forwarded to avatar")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("PNG"))}, nil
	})}}
	r := httptest.NewRequest("GET", g.avatarURL, nil)
	r.Header.Set("Authorization", "Bearer PRIVATE")
	r.Header.Set("Cookie", "PRIVATE")
	w := httptest.NewRecorder()
	g.serve(w, r)
	if w.Code != 200 || calls != 1 {
		t.Fatal("registered avatar failed")
	}
	r = httptest.NewRequest("GET", "https://lh3.googleusercontent.com/OTHER", nil)
	w = httptest.NewRecorder()
	g.serve(w, r)
	if w.Code != 403 || calls != 1 {
		t.Fatal("unregistered URL reached upstream")
	}
	b, _ := json.Marshal(g.report())
	if strings.Contains(string(b), "PRIVATE") || strings.Contains(string(b), "OTHER") {
		t.Fatal("avatar identity exported")
	}
}

func TestActualNativeProviderUsesProductionContractAndParsesSSE(t *testing.T) {
	report, err := nativeFixture()
	if err != nil {
		t.Fatal("network-none native fixture failed")
	}
	r := report["request"].(map[string]any)
	if report["fixture_final_verified"] != true || report["real_model_calls"] != 0 || r["host"] != "aicode.googleapis.com" || r["path"] != "/v1:streamGenerateContent" || r["model"] != "gemini-3.8-flash-tiered" || r["max_output_tokens"] != float64(8192) || r["fixture_prompt_present"] != true || r["session_id_present"] != false {
		t.Fatal("native contract or final incorrect")
	}
	if _, exists := r["tool_group_count"]; exists {
		t.Fatal("unexpected tool declaration")
	}
}

func TestMetadataRejectsCredentialSubstitutionBeforeUpstream(t *testing.T) {
	g := &metadataGateway{token: "PRIVATE-ORIGINAL", deadline: time.Now().Add(time.Minute)}
	r := httptest.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	r.Header.Set("Authorization", "Bearer PRIVATE-OTHER")
	w := httptest.NewRecorder()
	g.serve(w, r)
	if w.Code != 401 {
		t.Fatal("wrong account credential admitted")
	}
	b, _ := json.Marshal(g.report())
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("credential exported")
	}
}

func TestSyntheticGenerationAlwaysRejectsWithoutUpstream(t *testing.T) {
	f := &fixture{}
	r := httptest.NewRequest("POST", "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent", strings.NewReader(`{"model":"gemini-3.8-flash-high"}`))
	w := httptest.NewRecorder()
	f.serve(w, r)
	if w.Code != 429 || len(f.requests) != 1 {
		t.Fatal("generation was not captured and denied")
	}
	r = httptest.NewRequest("POST", "https://arbitrary.example/generate", nil)
	w = httptest.NewRecorder()
	f.serve(w, r)
	if w.Code != 403 || len(f.requests) != 1 {
		t.Fatal("arbitrary destination admitted")
	}
}

func TestMalformedAndOversizedCaptureDoesNotExportValues(t *testing.T) {
	r := httptest.NewRequest("POST", "https://aicode.googleapis.com/v1:generateContent", nil)
	v := projection(r, []byte(`not-json-PRIVATE`))
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("malformed body exported")
	}
	f := &fixture{}
	r = httptest.NewRequest("POST", "https://aicode.googleapis.com/v1:generateContent", strings.NewReader(strings.Repeat("x", 65537)))
	w := httptest.NewRecorder()
	f.serve(w, r)
	if w.Code != 413 || len(f.requests) != 0 {
		t.Fatal("oversized request admitted")
	}
}

func TestMetadataPolicyDeniesEveryGenerationAndWrite(t *testing.T) {
	for _, path := range []string{"/v1internal:streamGenerateContent", "/v1internal:generateContent", "/v1internal:tabChat", "/v1internal:streamGenerateChat", "/v1internal:generateCode", "/v1internal:countTokens", "/v1internal:onboardUser", "/v1internal:setUserSettings", "/token", "/unknown"} {
		if desktopMetadataAllowed("daily-cloudcode-pa.googleapis.com", path, "POST") {
			t.Fatalf("unsafe route admitted: %s", path)
		}
	}
	if desktopMetadataAllowed("arbitrary.example", "/v1internal:loadCodeAssist", "POST") || desktopMetadataAllowed("daily-cloudcode-pa.googleapis.com", "/v1internal:loadCodeAssist", "GET") {
		t.Fatal("host/method broadened")
	}
	if !desktopMetadataAllowed("daily-cloudcode-pa.googleapis.com", "/v1internal:listExperiments", "POST") {
		t.Fatal("required metadata missing")
	}
	g := &metadataGateway{deadline: time.Now().Add(time.Minute)}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://daily-cloudcode-pa.googleapis.com/v1internal:tabChat", strings.NewReader(`{"project":"PRIVATE","request":{"prompt":"PRIVATE"}}`))
	g.serve(w, r)
	if w.Code != 403 {
		t.Fatal("auxiliary call not denied")
	}
	b, _ := json.Marshal(g.report())
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("private profile exported")
	}
}
