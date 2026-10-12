package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func highFixture() []byte {
	b, _ := json.Marshal(map[string]any{"model": "gemini-3.8-flash-high", "project": "fixture-project", "userAgent": "antigravity", "requestType": "agent", "requestId": "agent-aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "request": map[string]any{"sessionId": strings.Repeat("b", 32), "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": comparePrompt}}}}, "generationConfig": map[string]any{"maxOutputTokens": 8192, "thinkingConfig": map[string]int{"thinkingBudget": -1}}}})
	return b
}
func highRequest(body []byte) *http.Request {
	r := httptest.NewRequest("POST", "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse", strings.NewReader(string(body)))
	for key, value := range map[string]string{"Authorization": "Bearer fixture-token", "Content-Type": "application/json", "Accept": "text/event-stream", "User-Agent": "antigravity/2.22.0 butler/0.1"} {
		r.Header.Set(key, value)
	}
	return r
}

type highMemoryAdmission struct {
	mu    sync.Mutex
	spent bool
}

func (a *highMemoryAdmission) Reserve(context.Context, string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.spent {
		return 0, denied
	}
	a.spent = true
	return 22, nil
}
func (*highMemoryAdmission) Observe(context.Context, int, observation) error { return nil }
func TestHighGateDeniesMutationThenSingleConcurrentWritePreservesBytes(t *testing.T) {
	body := highFixture()
	auth := providerauth.AntigravityAuth{AccessToken: "fixture-token", ProjectID: "fixture-project"}
	if !validHighRequest(highRequest(body), body, auth) {
		t.Fatal("production High fixture rejected")
	}
	for _, bad := range [][]byte{[]byte(strings.Replace(string(body), "8192", "65536", 1)), []byte(strings.Replace(string(body), "fixture-project", "foreign-project", 1)), []byte(strings.Replace(string(body), "flash-high", "flash-medium", 1)), []byte(strings.Replace(string(body), `"contents":`, `"tools":[{}],"contents":`, 1)), []byte(strings.Replace(string(body), `"sessionId":`, `"useG1Credits":true,"sessionId":`, 1)), []byte(strings.Replace(string(body), "Не вызывай инструменты.", "Injected wrapper instruction", 1)), []byte(strings.Replace(string(body), `"thinkingBudget":-1`, `"thinkingBudget":-1,"THINKINGBUDGET":100`, 1))} {
		if validHighRequest(highRequest(bad), bad, auth) {
			t.Fatal("changed or credit-enabled profile admitted")
		}
	}
	var calls atomic.Int32
	g := highGate{auth: auth, budget: &highMemoryAdmission{}, upstream: &http.Client{Transport: fakeRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		sent, _ := io.ReadAll(r.Body)
		if string(sent) != string(body) {
			t.Error("High bytes rewritten")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"" + marker + "\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":28,\"candidatesTokenCount\":17,\"totalTokenCount\":45}}}\n\n"))}, nil
	})}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); g.ServeHTTP(httptest.NewRecorder(), highRequest(body)) }()
	}
	wg.Wait()
	if calls.Load() != 1 || g.observation == nil || g.observation.Attempt != 22 || !g.observation.MarkerMatch {
		t.Fatal("High duplicate dispatched or observation lost")
	}
}
func TestHighPostgresCarries21AttemptsAndCannotReopen(t *testing.T) {
	pool := highTestPool(t, "high_fixture")
	defer pool.Close()
	ctx := context.Background()
	p := pgAdmission{pool: pool, root: "/src", high: true}
	if p.init(ctx) != nil {
		t.Fatal("High ledger init failed")
	}
	if _, err := p.Reserve(ctx, "cli"); err == nil {
		t.Fatal("CLI fallback admitted")
	}
	if id, err := p.Reserve(ctx, "go"); err != nil || id != 22 {
		t.Fatal("High reservation did not carry21")
	}
	if _, err := p.Reserve(ctx, "go"); err == nil || p.init(ctx) == nil {
		t.Fatal("High replay or restart reopened")
	}
	if p.Observe(ctx, 22, observation{Attempt: 22, Client: "go", Outcome: "UNKNOWN"}) != nil {
		t.Fatal("unknown observation persistence failed")
	}
	var raw []byte
	var spent, base, reserved int
	if pool.QueryRow(ctx, `SELECT historical,spent FROM diagnostic_window`).Scan(&base, &spent) != nil || base != 21 || spent != 1 {
		t.Fatal("historical accounting reset")
	}
	if pool.QueryRow(ctx, `SELECT observation,reserved_output_tokens FROM diagnostic_attempt WHERE id=22`).Scan(&raw, &reserved) != nil || reserved != 8192 || !strings.Contains(string(raw), `"input_tokens": null`) {
		t.Fatal("unknown usage or allocation lost")
	}
}
func TestHighExpiredPostgresWindowDenies(t *testing.T) {
	pool := highTestPool(t, "high_expired_fixture")
	defer pool.Close()
	ctx := context.Background()
	p := pgAdmission{pool: pool, root: "/src", high: true}
	if p.init(ctx) != nil {
		t.Fatal("High fixture init failed")
	}
	if _, err := pool.Exec(ctx, `UPDATE diagnostic_window SET deadline=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal("expiry fixture failed")
	}
	if _, err := p.Reserve(ctx, "go"); err == nil {
		t.Fatal("expired High request admitted")
	}
}
func highTestPool(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_COMPARE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("fixture pool failed")
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("fixture schema failed")
	}
	pool.Close()
	pool, err = pgxpool.New(ctx, dsn+"&search_path="+schema)
	if err != nil {
		t.Fatal("fixture schema pool failed")
	}
	return pool
}
