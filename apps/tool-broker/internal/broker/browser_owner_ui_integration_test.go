package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Chromium UI -> real authenticated owner HTTP/controller/goal service -> PG ->
// actual Broker process -> actual server Chromium. Only bootstrap data is seeded.
func TestBrowserSafetyOwnerUIChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_OWNER_UI_E2E") != "1" {
		t.Skip("isolated actual owner UI/PG/Chromium required")
	}
	readyPath := filepath.Join(t.TempDir(), "private-owner-ready.json")
	cmd := exec.Command(os.Getenv("BUTLER_BROWSER_OWNER_API_BINARY"), "-test.run=^TestBrowserSafetyOwnerAPIHarness$", "-test.timeout=6m")
	cmd.Env = append(os.Environ(), "BUTLER_BROWSER_SAFETY_OWNER_HARNESS=1", "BUTLER_BROWSER_SAFETY_OWNER_READY="+readyPath)
	// This fixture has only public synthetic state; preserve failures for diagnosis.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if cmd.Start() != nil {
		t.Fatal("owner API process unavailable")
	}
	var once sync.Once
	stop := func() { once.Do(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) }
	t.Cleanup(stop)
	var cfg struct {
		DSN, SearchPath, Address string
		Refs                     map[string]string
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, err := os.ReadFile(readyPath)
		if err == nil && json.Unmarshal(data, &cfg) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner API readiness timed out")
		}
		time.Sleep(25 * time.Millisecond)
	}
	pc, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		t.Fatal("private isolated PG config invalid")
	}
	pc.ConnConfig.RuntimeParams["search_path"] = cfg.SearchPath
	pc.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var posts atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == "POST" {
			posts.Add(1)
			fmt.Fprint(w, "saved")
			return
		}
		fmt.Fprint(w, `<form method=post action=/effect><button id=submit>Send</button></form>`)
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	f := resourceBrokerWithPool(t, nil, false, pool)
	client, ctx, _ := browserSafetyBrokerWithContext(t, f, cfg.Address+"/private/context")
	type step struct {
		Kind string `json:"kind"`
		Task string `json:"task"`
		API  string `json:"api"`
	}
	var mu sync.Mutex
	current := step{}
	proceed := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(current)
	})
	mux.HandleFunc("/continue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		select {
		case proceed <- r.URL.Query().Get("kind"):
			w.WriteHeader(200)
		default:
			w.WriteHeader(409)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:4182")
	if err != nil {
		t.Fatal(err)
	}
	coord := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	defer coord.Close()
	go func() { _ = coord.Serve(listener) }()
	for _, kind := range []string{"stop", "revoke", "control"} {
		runID := cfg.Refs[kind+"_run"]
		grant := func(callID, name, args string) *toolbrokerv1.ExecuteToolCallRequest {
			call := &toolbrokerv1.ToolCall{RunId: runID, ToolCallId: callID, ToolName: name, AutonomyMode: 3, ArgsJson: args}
			p, err := client.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
			if err != nil || p.GetError() != nil {
				t.Fatal("UI fixture prepare denied", err, p.GetError())
			}
			g, err := client.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: p.GetAction().GetActionId()})
			if err != nil || g.GetError() != nil || g.GetAuthorizationId() == "" {
				t.Fatal("UI fixture owner mandate denied", err, g.GetError())
			}
			return &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: g.AuthorizationId}
		}
		open := grant(kind+"-open", "browser.open", `{"url":"`+egresstest.Origin+`/form"}`)
		r, err := client.ExecuteToolCall(ctx, open)
		var view struct {
			ID      string `json:"browser_session_id"`
			URL     string `json:"final_url"`
			Version int64  `json:"observation_version"`
		}
		if err != nil || r.GetResult().GetStatus() != "completed" || json.Unmarshal([]byte(r.GetResult().GetResultJson()), &view) != nil || view.ID == "" {
			t.Fatal("owner UI browser open failed", r.GetResult().GetError())
		}
		original := grant(kind+"-click", "browser.click", fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, view.ID, view.URL, view.Version))
		if kind == "control" {
			r, err = client.ExecuteToolCall(ctx, original)
			if err != nil || r.GetResult().GetStatus() != "effect_unknown" || posts.Load() != 1 {
				t.Fatal("unchanged owner UI fixture cannot submit a POST", r.GetResult().GetError())
			}
			t.Log("owner_ui=unchanged_control recipient_posts=1 opaque_effect=effect_unknown")
			continue
		}
		mu.Lock()
		current = step{Kind: kind, Task: cfg.Refs[kind+"_task"], API: cfg.Address}
		mu.Unlock()
		select {
		case actual := <-proceed:
			if actual != kind {
				t.Fatal("UI advanced wrong case")
			}
		case <-time.After(2 * time.Minute):
			t.Fatal("owner UI did not apply control", kind)
		}
		r, err = client.ExecuteToolCall(ctx, original)
		if err != nil || r.GetResult().GetStatus() != "failed" || posts.Load() != 0 {
			t.Fatal("owner UI control allowed recipient POST", kind)
		}
		g, err := f.auth.GetGrant(ctx, original.AuthorizationId)
		var dispatches, receipts int
		if err != nil || g.State != "authorized" || !g.ExpiresAt.After(time.Now()) || pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&dispatches) != nil || dispatches != 0 {
			t.Fatal("UI denial changed original dispatch/grant")
		}
		if kind == "stop" {
			err = pool.QueryRow(ctx, `SELECT count(*) FROM task_control_events WHERE task_id=$1 AND kind='stop'`, cfg.Refs[kind+"_task"]).Scan(&receipts)
		} else {
			err = pool.QueryRow(ctx, `SELECT count(*) FROM owner_authority_events WHERE subject_id=$1 AND revision=2`, cfg.Refs[kind+"_mandate"]).Scan(&receipts)
		}
		if err != nil || receipts != 1 {
			t.Fatal("canonical owner receipt missing", kind, err)
		}
		t.Logf("owner_ui=%s real_owner_receipts=1 recipient_posts=0 original_grant=authorized original_dispatches=0", kind)
	}
	mu.Lock()
	current = step{Kind: "done"}
	mu.Unlock()
	// The current app polls owner observations in the background. Keep its API
	// alive until Playwright has drained routes and closed the page; otherwise
	// teardown can turn successful controls into a late connection-refused error.
	select {
	case <-proceed:
	case <-time.After(10 * time.Second):
		t.Fatal("UI final receipt not observed")
	}
	response, err := http.Post(cfg.Address+"/fixture/done", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
}
