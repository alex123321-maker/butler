package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This explicit operator gate starts the actual App.Run and Broker command, not
// a Core fixture. Native user turns are admitted only through the real poller or
// authenticated HTTP. Ordinary test/CI never reads credentials or calls a model.
func TestNativeCommunicationApplicationLab(t *testing.T) {
	if os.Getenv("BUTLER_COMMUNICATION_APPLICATION_LAB") != "1" {
		t.Skip("explicit native application lab required")
	}
	if os.Getenv("BUTLER_COMMUNICATION_AUTH_POSTGRES_URL") == "" || os.Getenv("BUTLER_COMMUNICATION_BROKER_COMMAND") == "" || os.Getenv("BUTLER_REDIS_URL") == "" {
		t.Fatal("private native service references required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	widgetLab := os.Getenv("BUTLER_COMMUNICATION_WIDGET_LAB") == "1"
	if widgetLab {
		cancel()
		duration := 30 * time.Minute
		if os.Getenv("BUTLER_COMMUNICATION_INPUT_LOOP_LAB") == "1" {
			// Allow manual inspection/approval of all eight discovery flows. The
			// initially fixed model/call budgets below do not change during a run.
			duration = time.Hour
		}
		ctx, cancel = context.WithTimeout(context.Background(), duration)
	}
	defer cancel()
	route := communicationNativeModelRoute(t)
	ctx = route.context(ctx)
	root := os.Getenv("BUTLER_COMMUNICATION_APPLICATION_PRIVATE")
	if root != "/private/application" {
		t.Fatal("fixed private lab directory required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal("private lab unavailable")
	}
	manifest := filepath.Join(root, "deletions.log")
	f := newDelegationGuardFixtureWithManifest(t, manifest)
	f.ctx = ctx
	defer func() {
		if err := captureCommunicationApplication(root, f.pool); err != nil {
			t.Error("private application evidence capture failed")
		}
	}()
	dsn, err := url.Parse(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal("isolated database unavailable")
	}
	query := dsn.Query()
	query.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	t.Setenv("BUTLER_POSTGRES_URL", dsn.String())
	f.budgets.Policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	f.budgets.Policy.Day.ModelTurns = 12
	f.budgets.Policy.Day.Tokens = 12 * (1048576 + 8192)
	f.budgets.Policy.Conversation.ModelTurns = 8
	f.budgets.Policy.Conversation.Tokens = 8 * (1048576 + 8192)
	if widgetLab {
		f.budgets.Policy.Day.ModelTurns = 32
		f.budgets.Policy.Day.Tokens = 32 * (1048576 + 8192)
		f.budgets.Policy.Conversation.ModelTurns = 32
		f.budgets.Policy.Conversation.Tokens = 32 * (1048576 + 8192)
	}
	// Eight ordinary requests (four per channel) exercise the old session-wide
	// discovery repetition limit. This is one finite isolated window, not a
	// per-request refill or a change to the working owner profile.
	if os.Getenv("BUTLER_COMMUNICATION_INPUT_LOOP_LAB") == "1" {
		if !widgetLab || os.Getenv("BUTLER_TOOL_DISCOVERY_ENABLED") != "true" {
			t.Fatal("input-loop lab requires widgets and actual tool discovery")
		}
		f.budgets.Policy.Day.ModelTurns = 64
		f.budgets.Policy.Day.Tokens = 64 * (1048576 + 8192)
		f.budgets.Policy.Conversation.ActiveMillis = 2 * 60 * 60 * 1000
	}
	policy, _ := json.Marshal(f.budgets.Policy)
	t.Setenv("BUTLER_EXECUTION_BUDGET_POLICY_JSON", string(policy))
	for key, value := range map[string]string{
		"BUTLER_HTTP_ADDR": "0.0.0.0:18470", "BUTLER_GRPC_ADDR": "127.0.0.1:19470", "BUTLER_TOOL_BROKER_ADDR": "127.0.0.1:19471",
		"BUTLER_MODEL_PROVIDER": "antigravity", "BUTLER_ANTIGRAVITY_MODEL": "gemini-3.8-flash-high", "BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS": "8192",
		"BUTLER_OPENAI_TIMEOUT_SECONDS": "90", "BUTLER_AUTONOMY_MODE": "mode_2", "BUTLER_MEMORY_PIPELINE_ENABLED": "false",
		"BUTLER_MEMORY_DELETION_MANIFEST_FILE": manifest, "BUTLER_OWNER_CONTROL_OWNER_ID": f.parent.OwnerID,
	} {
		t.Setenv(key, value)
	}
	writeProof := func(path string) {
		var data [32]byte
		if _, e := rand.Read(data[:]); e != nil {
			t.Fatal("private proof unavailable")
		}
		if e := os.WriteFile(path, []byte(hex.EncodeToString(data[:])), 0600); e != nil {
			t.Fatal("private proof unavailable")
		}
	}
	ownerFile, serviceFile := filepath.Join(root, "owner-proof"), filepath.Join(root, "service-proof")
	writeProof(ownerFile)
	writeProof(serviceFile)
	t.Setenv("BUTLER_OWNER_CONTROL_TOKEN_FILE", ownerFile)
	t.Setenv("BUTLER_ACTION_AUTHORITY_TOKEN_FILE", serviceFile)
	registry := filepath.Join(root, "tools.json")
	if err = os.WriteFile(registry, []byte(`{"tools":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUTLER_TOOL_REGISTRY_PATH", registry)
	authPool, err := pgxpool.New(ctx, os.Getenv("BUTLER_COMMUNICATION_AUTH_POSTGRES_URL"))
	if err != nil {
		t.Fatal("canonical auth unavailable")
	}
	defer authPool.Close()
	chat, err := strconv.ParseInt(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_CHAT_ID"), 10, 64)
	if err != nil {
		t.Fatal("test chat required")
	}
	client, err := telegramrich.NewClientFromTokenFileWithProxyFile(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE"), os.Getenv("BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE"))
	if err != nil {
		t.Fatal("test bot unavailable")
	}
	// Explicitly discard only pre-existing updates of this dedicated test bot,
	// before any worker starts. A fresh lab never executes historical instructions.
	var offset int64
	for n := 0; n < 4; n++ {
		updates, e := client.Updates(ctx, offset)
		if e != nil {
			t.Fatal("test bot backlog preflight unavailable")
		}
		for _, u := range updates {
			if u.ID >= offset {
				offset = u.ID + 1
			}
		}
		if len(updates) < 100 {
			break
		}
		if n == 3 {
			t.Fatal("test bot backlog exceeds bounded preflight")
		}
	}
	options := TelegramCommunicationOptions{
		OwnerID: f.parent.OwnerID, BindingID: "application-telegram", WebBindingID: "application-web", ChatID: chat,
		ExpectedBot: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_BOT"), TokenFile: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE"), BotAPIProxyFile: os.Getenv("BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE"), AllowPrivate: true,
		ProviderAuthStore: communicationProviderAuth{store: config.NewPostgresSettingsStore(authPool)},
		Widgets:           widgetLab,
	}
	app, err := NewForTelegramCommunication(ctx, options)
	if err != nil {
		t.Fatal("actual application construction failed:", err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE communication_telegram_cursors SET next_update_id=$1 WHERE binding_id='application-telegram' AND next_update_id=0`, offset); err != nil {
		t.Fatal("private initial cursor unavailable")
	}
	seedCtx := f.leasedWorker(t, f.parent, "application-media-fixture")
	canvas := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 170, A: 255})
		}
	}
	var picture bytes.Buffer
	if err = png.Encode(&picture, canvas); err != nil {
		t.Fatal(err)
	}
	media := artifacts.NewService(artifacts.NewPostgresRepository(f.pool))
	im, err := media.SaveMedia(seedCtx, "application-media-fixture", f.parent.SessionKey, "shared-image.png", "image/png", picture.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := media.SaveMedia(seedCtx, "application-media-fixture", f.parent.SessionKey, "shared-report.txt", "text/plain", []byte("Butler shared Web + Telegram fixture\nUnicode: Привет 🧪\n"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures, _ := json.Marshal(map[string]string{"image_ref": im, "document_ref": doc})
	if err = os.WriteFile(filepath.Join(root, "fixtures.json"), fixtures, 0600); err != nil {
		t.Fatal(err)
	}
	brokerLog, err := os.OpenFile(filepath.Join(root, "broker.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("private Broker log unavailable")
	}
	defer brokerLog.Close()
	arguments := []string{"-control-url", "http://127.0.0.1:18470/internal/communication"}
	if widgetLab {
		arguments = append(arguments, "-widgets")
	}
	command := exec.CommandContext(ctx, os.Getenv("BUTLER_COMMUNICATION_BROKER_COMMAND"), arguments...)
	command.Env = append(os.Environ(), "BUTLER_HTTP_ADDR=127.0.0.1:18471", "BUTLER_GRPC_ADDR=127.0.0.1:19471", "BUTLER_ACTION_CONTEXT_URL=http://127.0.0.1:18470/internal/action-context")
	command.Stdout = brokerLog
	command.Stderr = brokerLog
	if err = command.Start(); err != nil {
		t.Fatal("actual Broker command unavailable")
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	start := func(a *App) (context.CancelFunc, <-chan error) {
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- a.Run(runCtx) }()
		return stop, done
	}
	stopApp, result := start(app)
	defer func() {
		stopApp()
		if result == nil {
			return
		}
		select {
		case err := <-result:
			if err != nil {
				t.Error("application shutdown", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("application shutdown timeout")
		}
	}()
	probe := http.Client{Timeout: time.Second}
	for n := 0; n < 100; n++ {
		response, e := probe.Get("http://127.0.0.1:18470/health")
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("application startup timed out")
		case <-time.After(100 * time.Millisecond):
		}
		if n == 99 {
			t.Fatal("actual application did not start")
		}
	}
	if err = os.WriteFile(filepath.Join(root, "ready"), []byte("actual App.Run + private Broker ready\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("actual application ready; only real owner HTTP/Telegram inputs can start inference")
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	restarted := false
	for {
		select {
		case <-ctx.Done():
			t.Fatal("bounded application lab expired")
		case <-tick.C:
			if _, e := os.Stat(filepath.Join(root, "restart")); e == nil && !restarted {
				var active, before int
				if e = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running')),(SELECT count(*) FROM execution_model_attempts)`).Scan(&active, &before); e != nil {
					t.Fatal("restart admission unavailable")
				}
				if active > 0 {
					continue
				}
				stopApp()
				select {
				case e = <-result:
					result = nil
					if e != nil {
						t.Fatal("restart shutdown failed", e)
					}
				case <-time.After(15 * time.Second):
					t.Fatal("restart shutdown timed out")
				}
				if _, e = os.Stat(filepath.Join(root, "drain")); e == nil {
					options.DrainOnly = true
				}
				app, e = NewForTelegramCommunication(ctx, options)
				if e != nil {
					t.Fatal("same-profile application restart failed", e)
				}
				stopApp, result = start(app)
				restarted = true
				if e = os.Remove(filepath.Join(root, "restart")); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(root, "restarted"), []byte("same database/bindings/cursor; model attempt count at restart: "+strconv.Itoa(before)+"\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if _, err = os.Stat(filepath.Join(root, "stop")); err == nil {
				return
			}
		}
	}
}

func captureCommunicationApplication(root string, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var proof, retained []byte
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object('publications',(SELECT count(*) FROM communication_publications),'inputs',(SELECT count(*) FROM accepted_inputs),'model_attempts',(SELECT count(*) FROM execution_model_attempts),'observed_attempts',(SELECT count(actual_input_tokens) FROM execution_model_attempts),'unobserved_attempts',(SELECT count(*)-count(actual_input_tokens) FROM execution_model_attempts),'telegram_sent',(SELECT count(*) FROM delivery_outbox WHERE channel='telegram' AND status='sent'),'web_available',(SELECT count(*) FROM delivery_outbox WHERE channel='web' AND status='result_ready'))`).Scan(&proof)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "summary.json"), proof, 0600); err != nil {
		return err
	}
	err = pool.QueryRow(ctx, `SELECT jsonb_build_object('publications',(SELECT coalesce(jsonb_agg(to_jsonb(p)),'[]') FROM communication_publications p),'deliveries',(SELECT coalesce(jsonb_agg(to_jsonb(d)),'[]') FROM delivery_outbox d),'messages',(SELECT coalesce(jsonb_agg(to_jsonb(m)),'[]') FROM messages m),'inputs',(SELECT coalesce(jsonb_agg(to_jsonb(i)),'[]') FROM accepted_inputs i),'model_attempts',(SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]') FROM execution_model_attempts a),'artifacts',(SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]') FROM artifacts a),'bindings',(SELECT coalesce(jsonb_agg(to_jsonb(b)),'[]') FROM communication_bindings b),'cursors',(SELECT coalesce(jsonb_agg(to_jsonb(c)),'[]') FROM communication_telegram_cursors c),'effects',(SELECT coalesce(jsonb_agg(to_jsonb(e)),'[]') FROM action_effects e),'interactions',(SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM communication_interactions x),'widget_views',(SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM communication_widget_views x),'widget_actions',(SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM communication_widget_actions x),'widget_projection_ops',(SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM communication_widget_projection_ops x),'runs',(SELECT coalesce(jsonb_agg(to_jsonb(r)),'[]') FROM runs r))`).Scan(&retained)
	if err != nil {
		return err
	}
	var records map[string]json.RawMessage
	if err = json.Unmarshal(retained, &records); err != nil {
		return err
	}
	// Retain the exact pre-existing keys, spent allowances and owning admissions
	// so the no-reset check survives disposal of the isolated schema. Fixed tables
	// only; never capture system_settings or provider credentials.
	for _, table := range []string{"execution_loop_observations", "execution_budget_scopes", "execution_budget_reservations", "communication_admissions", "tool_calls"} {
		var data []byte
		if err = pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM `+table+` x`).Scan(&data); err != nil {
			return err
		}
		records[table] = data
	}
	retained, err = json.Marshal(records)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "retained-evidence.json"), retained, 0600)
}
