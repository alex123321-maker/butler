// effort-eval runs an explicitly enabled synthetic research trial on this PC.
// It has no production entrypoint, mail connector, tool executor or delivery API.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/butler/butler/internal/acceptance/effort"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `CREATE TABLE IF NOT EXISTS system_settings(key text PRIMARY KEY,value text NOT NULL,component text NOT NULL,is_secret boolean NOT NULL,updated_at timestamptz NOT NULL,updated_by text NOT NULL);
CREATE TABLE IF NOT EXISTS effort_windows(id text PRIMARY KEY,manifest text NOT NULL,manifest_sha256 text NOT NULL,expires_at timestamptz NOT NULL,max_calls integer NOT NULL,calls integer NOT NULL DEFAULT 0,max_reserved bigint NOT NULL,reserved bigint NOT NULL DEFAULT 0,closed boolean NOT NULL DEFAULT false);
CREATE TABLE IF NOT EXISTS effort_attempts(window_id text NOT NULL REFERENCES effort_windows(id),n integer NOT NULL,slot text NOT NULL,profile text NOT NULL,input text NOT NULL,input_sha256 text NOT NULL,status text NOT NULL DEFAULT 'unknown',output text,reason text,input_tokens bigint,output_tokens bigint,total_tokens bigint,model_version text,milliseconds bigint,started_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(window_id,n));
ALTER TABLE effort_attempts ADD COLUMN IF NOT EXISTS thinking_tokens bigint;
ALTER TABLE effort_attempts ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'admitted';
ALTER TABLE effort_attempts ADD COLUMN IF NOT EXISTS http_status integer;
ALTER TABLE effort_attempts ADD COLUMN IF NOT EXISTS safe_reason text;
CREATE TABLE IF NOT EXISTS effort_states(window_id text NOT NULL REFERENCES effort_windows(id),slot text NOT NULL,n bigint GENERATED ALWAYS AS IDENTITY,state jsonb NOT NULL,at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(window_id,n));
CREATE TABLE IF NOT EXISTS effort_outcomes(window_id text NOT NULL REFERENCES effort_windows(id),slot text NOT NULL,outcome jsonb NOT NULL,milliseconds bigint NOT NULL,PRIMARY KEY(window_id,slot));`

type options struct{ action, keyFile, importFile, out, trial, source, clientFile, loginURLFile string }

func main() {
	var o options
	flag.StringVar(&o.action, "action", "inventory", "prepare, inventory, run, export")
	flag.StringVar(&o.keyFile, "key-file", "", "private encryption key file")
	flag.StringVar(&o.importFile, "import-file", "", "private operator import file, prepare only")
	flag.StringVar(&o.out, "out", "", "new evidence directory, run/export only")
	flag.StringVar(&o.trial, "trial", "", "new immutable trial ID, run/export only")
	flag.StringVar(&o.source, "source", "", "prospectively frozen source identity")
	flag.StringVar(&o.clientFile, "oauth-client-file", "", "private native OAuth application file, login only")
	flag.StringVar(&o.loginURLFile, "login-url-file", "", "private output file for local browser login URL")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := execute(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, o options) error {
	if o.action == "summarize" {
		return summarize(o.out)
	}
	if os.Getenv("BUTLER_EFFORT_LAB_ENABLE") != "synthetic-local-only" {
		return errors.New("explicit synthetic local lab enablement required")
	}
	dsn := os.Getenv("BUTLER_EFFORT_LAB_POSTGRES_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") || u.Path != "/effort97_test" {
		return errors.New("isolated loopback effort97_test database required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("local database configuration failed")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, schema); err != nil {
		return errors.New("local research schema unavailable")
	}
	key, err := os.ReadFile(o.keyFile)
	if err != nil || len(bytes.TrimSpace(key)) < 32 {
		return errors.New("private encryption key file unavailable")
	}
	defer clear(key)
	m := providerauth.NewManager(config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	switch o.action {
	case "login":
		return localLogin(ctx, m, o.clientFile, o.loginURLFile)
	case "prepare":
		f, e := os.Open(o.importFile)
		if e != nil {
			return errors.New("private import file unavailable")
		}
		defer f.Close()
		if e = m.ImportAntigravity(ctx, f, "issue97-local-research"); e != nil {
			return e
		}
		fmt.Println("Private provider import encrypted in local PostgreSQL.")
		return nil
	case "inventory":
		x, e := inventory(ctx, m)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(x)
	case "run", "preflight":
		return runTrial(ctx, pool, m, o)
	case "evidence":
		return runEvidenceTrial(ctx, pool, m, o)
	case "export":
		return exportTrial(ctx, pool, o)
	default:
		return errors.New("unknown local research action")
	}
}

type modelInventory struct {
	ObservedAt time.Time `json:"observed_at"`
	HTTPStatus int       `json:"http_status"`
	Models     []string  `json:"models"`
	Evidence   string    `json:"evidence"`
}

func inventory(ctx context.Context, m *providerauth.Manager) (modelInventory, error) {
	x := modelInventory{ObservedAt: time.Now().UTC(), Evidence: "authenticated_model_names_only_not_effective_reasoning_proof"}
	a, err := m.ResolveAntigravityCloudCode(ctx)
	if err != nil {
		return x, errors.New("private paid subscription discovery failed")
	}
	b, _ := json.Marshal(map[string]string{"project": a.ProjectID})
	defer clear(b)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels", bytes.NewReader(b))
	if err != nil {
		return x, errors.New("inventory request failed")
	}
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := client.Do(req)
	if err != nil {
		return x, errors.New("inventory connection failed")
	}
	defer r.Body.Close()
	x.HTTPStatus = r.StatusCode
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024+1))
	defer clear(raw)
	if err != nil || len(raw) > 2*1024*1024 {
		return x, errors.New("inventory response invalid")
	}
	var body struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if r.StatusCode != 200 || json.Unmarshal(raw, &body) != nil {
		return x, errors.New("inventory unavailable")
	}
	for name := range body.Models {
		// Only bounded public model identifiers leave the private control plane.
		if len(name) > 128 || !strings.HasPrefix(name, "gemini-") {
			continue
		}
		valid := true
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
				valid = false
			}
		}
		if valid {
			x.Models = append(x.Models, name)
		}
	}
	sort.Strings(x.Models)
	return x, nil
}

type slot struct {
	Case    string      `json:"case"`
	Profile string      `json:"profile"`
	Mode    effort.Mode `json:"mode"`
	Repeat  int         `json:"repeat"`
}

func (s slot) ID() string { return fmt.Sprintf("%s/%s/%s/%d", s.Case, s.Profile, s.Mode, s.Repeat) }

type manifest struct {
	Runtime        string            `json:"runtime"`
	Protocol       string            `json:"protocol"`
	Source         string            `json:"source"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Inventory      modelInventory    `json:"inventory"`
	Instructions   string            `json:"instructions"`
	ResponseSchema json.RawMessage   `json:"response_schema"`
	ThinkingLevels map[string]string `json:"explicit_thinking_levels"`
	Cases          []effort.Case     `json:"cases"`
	Slots          []slot            `json:"slots"`
	MaxTaskCalls   int               `json:"max_task_calls"`
	InputByteCap   int               `json:"input_byte_cap"`
	OutputTokenCap int               `json:"output_token_cap"`
	TaskSeconds    int               `json:"task_seconds"`
	Repeats        int               `json:"repeats"`
	Grading        string            `json:"grading"`
	Limitations    []string          `json:"limitations"`
}

func newManifest(source string, inv modelInventory) manifest {
	now := time.Now().UTC()
	f := manifest{Protocol: "issue97-synthetic-v2", Source: source, CreatedAt: now, ExpiresAt: now.Add(3 * time.Hour), Inventory: inv, Instructions: effort.Instructions, ResponseSchema: effort.NativeSchemaV2, ThinkingLevels: map[string]string{antigravity.FlashLowModel: "LOW", antigravity.FlashHighModel: "HIGH"}, Cases: effort.CasesV1(), MaxTaskCalls: 6, InputByteCap: 65536, OutputTokenCap: 8192, TaskSeconds: 300, Repeats: 1, Grading: "fixed off-model decision/value/required-source facets; failures included; free-text manual review separate", Limitations: []string{"synthetic fixture reads, no real mail or external tool execution", "lab baseline uses explicit JSON decisions, not a production end-to-end acceptance", "subscription currency/quota cost unknown; missing thinking counts remain null", "one repeat per case; descriptive pilot, no equivalence or universal superiority claim"}}
	profiles := []string{antigravity.FlashLowModel, antigravity.FlashHighModel}
	f.Runtime = runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH
	f.Limitations = append(f.Limitations, "advertised Low/High profiles with explicit LOW/HIGH on the same paid route do not disclose weights or exact hidden reasoning budgets")
	modes := []effort.Mode{effort.Baseline, effort.Repeat, effort.CheckSame, effort.Checked}
	for i, c := range f.Cases {
		for j := range modes {
			m := modes[(i+j)%len(modes)]
			for k := range profiles {
				p := profiles[(i+j+k)%len(profiles)]
				f.Slots = append(f.Slots, slot{c.ID, p, m, 0})
			}
		}
	}
	return f
}

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func runTrial(ctx context.Context, pool *pgxpool.Pool, m *providerauth.Manager, o options) error {
	if o.trial == "" || len(o.trial) > 100 || strings.ContainsAny(o.trial, "/\\ \r\n") || o.source == "" || o.out == "" {
		return errors.New("new trial, source identity and evidence directory required")
	}
	if _, err := os.Stat(o.out); !os.IsNotExist(err) {
		return errors.New("evidence directory already exists or cannot be checked")
	}
	inv, err := inventory(ctx, m)
	if err != nil {
		return err
	}
	f := newManifest(o.source, inv)
	for _, required := range []string{antigravity.FlashLowModel, antigravity.FlashHighModel} {
		found := false
		for _, name := range inv.Models {
			if name == required {
				found = true
			}
		}
		if !found {
			return errors.New("comparable Low/High inventory unavailable; do not substitute profiles")
		}
	}
	if o.action == "preflight" {
		f.Protocol = "issue97-profile-preflight-v2"
		f.Cases = []effort.Case{{ID: "profile-preflight", Group: "preflight_not_quality_sample", Goal: "Верни decision=answer, value=ready, answer=Готово, goal_version=1. Остальные массивы пустые. Это проверка протокола JSON, а не тест качества.", Expected: effort.Expected{Decision: "answer", Value: "ready"}}}
		f.Slots = []slot{{Case: f.Cases[0].ID, Profile: antigravity.FlashLowModel, Mode: effort.Baseline}, {Case: f.Cases[0].ID, Profile: antigravity.FlashHighModel, Mode: effort.Baseline}}
		f.MaxTaskCalls = 1
	}
	encoded, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Freeze and commit all slots/inputs/rubric before any trial inference.
	if _, err = pool.Exec(ctx, `INSERT INTO effort_windows(id,manifest,manifest_sha256,expires_at,max_calls,max_reserved) VALUES($1,$2,$3,$4,$5,$6)`, o.trial, string(encoded), digest(encoded), f.ExpiresAt, len(f.Slots)*f.MaxTaskCalls, int64(len(f.Slots)*f.MaxTaskCalls*(f.InputByteCap+f.OutputTokenCap))); err != nil {
		return errors.New("new immutable window could not be admitted")
	}
	if err = os.MkdirAll(o.out, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(o.out, "manifest.json"), encoded, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, f.ExpiresAt)
	defer cancel()
	defer func() {
		clean, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(clean, `UPDATE effort_windows SET closed=true WHERE id=$1`, o.trial)
		_ = exportTrial(clean, pool, o)
	}()
	for i, sl := range f.Slots {
		if ctx.Err() != nil {
			return errors.New("trial stopped")
		}
		var c effort.Case
		for _, x := range f.Cases {
			if x.ID == sl.Case {
				c = x
			}
		}
		start := time.Now()
		out := effort.Run(ctx, c, effort.Config{Mode: sl.Mode, MaxCalls: f.MaxTaskCalls, Deadline: time.Now().Add(time.Duration(f.TaskSeconds) * time.Second)}, func(call context.Context, s effort.State) (effort.Reply, error) {
			return sample(call, pool, m, o.trial, sl, c, s, f)
		}, func(call context.Context, s effort.State) error {
			b, e := json.Marshal(s)
			if e != nil {
				return e
			}
			_, e = pool.Exec(call, `INSERT INTO effort_states(window_id,slot,state) VALUES($1,$2,$3::jsonb)`, o.trial, sl.ID(), string(b))
			return e
		})
		b, _ := json.Marshal(out)
		if _, err = pool.Exec(ctx, `INSERT INTO effort_outcomes(window_id,slot,outcome,milliseconds) VALUES($1,$2,$3::jsonb,$4)`, o.trial, sl.ID(), string(b), time.Since(start).Milliseconds()); err != nil {
			return errors.New("outcome retention failed")
		}
		fmt.Printf("%d/%d %s %s correct=%t calls=%d reason=%s\n", i+1, len(f.Slots), sl.ID(), out.Status, out.Correct, out.State.Calls, out.Reason)
	}
	return nil
}

func sample(ctx context.Context, pool *pgxpool.Pool, m *providerauth.Manager, trial string, sl slot, c effort.Case, s effort.State, f manifest) (reply effort.Reply, resultErr error) {
	b, err := effort.ModelInput(c, s)
	if err != nil {
		return reply, err
	}
	n, err := reserveAttempt(ctx, pool, trial, sl.ID(), sl.Profile, b, f.InputByteCap+f.OutputTokenCap)
	if err != nil {
		return reply, err
	}
	return inferAttempt(ctx, pool, m, trial, n, sl, b, f)
}

func reserveAttempt(ctx context.Context, pool *pgxpool.Pool, trial, slotID, profile string, b []byte, units int) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var n int
	if units <= 0 {
		return 0, errors.New("invalid reservation")
	}
	err = tx.QueryRow(ctx, `UPDATE effort_windows SET calls=calls+1,reserved=reserved+$2 WHERE id=$1 AND NOT closed AND expires_at>clock_timestamp() AND calls<max_calls AND reserved+$2<=max_reserved RETURNING calls`, trial, units).Scan(&n)
	if err != nil {
		return 0, errors.New("finite trial window exhausted or closed")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO effort_attempts(window_id,n,slot,profile,input,input_sha256) VALUES($1,$2,$3,$4,$5,$6)`, trial, n, slotID, profile, string(b), digest(b)); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

func inferAttempt(ctx context.Context, pool *pgxpool.Pool, m *providerauth.Manager, trial string, n int, sl slot, b []byte, f manifest) (reply effort.Reply, resultErr error) {
	started := time.Now()
	var usage *transport.Usage
	version := ""
	raw := ""
	reason := "incomplete"
	status := "unknown"
	var stage atomic.Value
	stage.Store("construction")
	var httpStatus int
	safeReason := ""
	defer func() {
		retain, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		var in, out, total *int
		if usage != nil {
			in = &usage.InputTokens
			out = &usage.OutputTokens
			total = &usage.TotalTokens
		}
		tag, e := pool.Exec(retain, `UPDATE effort_attempts SET status=$3,output=$4,reason=$5,input_tokens=$6,output_tokens=$7,total_tokens=$8,model_version=NULLIF($9,''),milliseconds=$10,thinking_tokens=$11,stage=$12,http_status=NULLIF($13,0),safe_reason=NULLIF($14,'') WHERE window_id=$1 AND n=$2 AND status='unknown'`, trial, n, status, raw, reason, in, out, total, version, time.Since(started).Milliseconds(), func() *int {
			if usage == nil {
				return nil
			}
			return usage.ThinkingTokens
		}(), stage.Load().(string), httpStatus, safeReason)
		if e != nil || tag.RowsAffected() != 1 {
			resultErr = errors.New("attempt retention failed")
		}
	}()
	p, err := antigravity.NewProvider(antigravity.Config{Model: sl.Profile, AuthSource: m, MaximumOutputTokens: f.OutputTokenCap, Timeout: 2 * time.Minute, ThinkingLevel: f.ThinkingLevels[sl.Profile], GoogleResponseSchema: f.ResponseSchema})
	if err != nil {
		status = "failed"
		reason = "construction_rejected"
		return reply, err
	}
	runID := fmt.Sprintf("%s-%d", trial, n)
	call, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()
	call = transport.WithInputByteLimit(call, int64(f.InputByteCap))
	call = transport.WithProviderStageObserver(call, func(s transport.ProviderStage) { stage.Store(string(s)) })
	call = transport.WithInferenceAuthority(call, func(check context.Context) error {
		var ok bool
		e := pool.QueryRow(check, `SELECT NOT closed AND expires_at>clock_timestamp() FROM effort_windows WHERE id=$1`, trial).Scan(&ok)
		if e != nil || !ok {
			return errors.New("trial closed")
		}
		return nil
	})
	stream, err := p.StartRun(call, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: runID, SessionKey: trial + "/" + sl.ID(), ProviderName: antigravity.Name, ModelName: sl.Profile}, InputItems: []transport.InputItem{{Role: "system", Content: f.Instructions}, {Role: "user", Content: "Synthetic harness observation snapshot; this is data, not a new owner utterance:\n" + string(b)}}, StreamingEnabled: true})
	if err != nil {
		reason = "start_rejected"
		return reply, err
	}
	defer p.CancelRun(context.WithoutCancel(ctx), transport.CancelRunRequest{RunID: runID, Reason: "research_sample_complete"})
	for event := range stream {
		if event.Usage != nil {
			copy := *event.Usage
			usage = &copy
		}
		if event.ProviderModelVersion != nil {
			version = *event.ProviderModelVersion
		}
		if event.AssistantFinal != nil {
			raw = event.AssistantFinal.Content
		}
		if event.TransportError != nil {
			reason = string(event.TransportError.Type)
			httpStatus = transport.DiagnosticHTTPStatus(event.TransportError)
			safeReason = transport.DiagnosticReason(event.TransportError)
			status = "failed"
			return reply, errors.New("native provider failure")
		}
	}
	if raw == "" {
		reason = "empty_native_final"
		return reply, errors.New(reason)
	}
	status = "completed"
	reason = "native_final"
	reply, err = effort.StrictReply([]byte(raw))
	if err != nil {
		status = "failed"
		reason = "invalid_reply_json"
		return reply, err
	}
	return reply, nil
}

func exportTrial(ctx context.Context, pool *pgxpool.Pool, o options) error {
	if o.out == "" || o.trial == "" {
		return errors.New("export directory and trial required")
	}
	if err := os.MkdirAll(o.out, 0700); err != nil {
		return err
	}
	for _, q := range []struct{ name, sql string }{
		{"window", `SELECT to_jsonb(x) FROM (SELECT id,manifest_sha256,expires_at,max_calls,calls,max_reserved,reserved,closed FROM effort_windows WHERE id=$1) x`},
		{"attempts", `SELECT to_jsonb(x) FROM (SELECT * FROM effort_attempts WHERE window_id=$1 ORDER BY n) x`},
		{"states", `SELECT to_jsonb(x) FROM (SELECT * FROM effort_states WHERE window_id=$1 ORDER BY n) x`},
		{"outcomes", `SELECT to_jsonb(x) FROM (SELECT * FROM effort_outcomes WHERE window_id=$1 ORDER BY slot) x`},
	} {
		rows, err := pool.Query(ctx, q.sql, o.trial)
		if err != nil {
			return err
		}
		var data bytes.Buffer
		for rows.Next() {
			var b []byte
			if err = rows.Scan(&b); err != nil {
				rows.Close()
				return err
			}
			data.Write(b)
			data.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(o.out, q.name+".jsonl"), data.Bytes(), 0600); err != nil {
			return err
		}
	}
	var frozen string
	if err := pool.QueryRow(ctx, `SELECT manifest FROM effort_windows WHERE id=$1`, o.trial).Scan(&frozen); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(o.out, "manifest.json"), []byte(frozen), 0600)
}
