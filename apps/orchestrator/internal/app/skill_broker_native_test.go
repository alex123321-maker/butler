package app

// Targeted opt-in native pipeline proof, separate from all closed ABC histories.
// Fixture owner approvals and the provenance setup are scripted; only the new
// consuming task uses Gemini. Every consuming tool is the real Broker route.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const skillBrokerTrial = "skill-broker-20261029-v1"
const skillBrokerTarget = "postgres://postgres@127.0.0.1:55455/skill_broker_prompt_v1_test?sslmode=disable"

var skillBrokerOldReceipts = map[string]string{
	"abc.json":     "8207969442551ef647bfa9a14919d7f325ebfd30f583db7f699e15a5d33160ad",
	"force.json":   "22702b0cd4fe7ecf30eac26a4f16e190b921c8ba4e80bcc223f724e537936a8d",
	"prompt.json":  "1929bff06e9ce83d5820ae6511424bdb1dc1c9bb5aff928006e97a99e20d0f5c",
	"context.json": "457ece9931bb89793e61b17d3bb0f4177c2d7dd4aebdd70feec2c7f19030df8b",
}

type skillBrokerTurn struct {
	Sequence      int
	Kind          string
	Request       any
	Started       time.Time
	ElapsedMillis int64
	Dispatched    bool
	Usage         *transport.Usage
	ModelVersion  *string
	Proposals     []transport.ToolCallRequest
	Final         string
	ErrorCode     string
}
type skillBrokerNative struct {
	control  *pgx.Conn
	authPool *pgxpool.Pool
	manager  *providerauth.Manager
	turns    []*skillBrokerTurn
	source   string
	failed   bool
	verified bool
	dbReady  bool
}

func skillBrokerWrite(name string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join("/out", name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(append(b, '\n')); e != nil {
		return e
	}
	return f.Sync()
}
func skillBrokerPlan() map[string]any {
	return map[string]any{"trial": skillBrokerTrial, "profile": antigravity.FlashHighModel, "maximum_native_methods": 12, "request_seconds": 90, "window_seconds": 900, "maximum_output_tokens": 4096, "target": skillBrokerTarget, "old_receipts": skillBrokerOldReceipts, "scope": "One native consuming README patch task through real Input/Session/Run/Core/Broker/runtime/recipient and durable synthetic owner approvals. A scripted actual-Broker baseline supplies current evidence to stage, test and activate one skill in this fresh database.", "required": "real completed skills.load plus PostgreSQL usage pin, exact digest, current procedure DATA restored across durable approvals, exactly one native prepared patch with confirmed effect, matching checked artifact and unchanged original; model final remains unverified until fixture owner verification", "stop": "Any native transport, authority, storage, accounting or tool failure stops; no operator retry or fallback", "retention": "All old histories and diagnostics immutable; this is separate targeted pipeline evidence, not a new ABC admission or issue closure"}
}
func newSkillBrokerNative(t *testing.T) *skillBrokerNative {
	t.Helper()
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("fixed laba proxy required")
	}
	for _, k := range []string{"https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		if os.Getenv(k) != "" {
			t.Fatal("alternate route denied")
		}
	}
	for name, want := range skillBrokerOldReceipts {
		raw, e := os.ReadFile(filepath.Join("/previous", name))
		h := sha256.Sum256(raw)
		if e != nil || hex.EncodeToString(h[:]) != want {
			t.Fatal("old evidence changed")
		}
	}
	binaryPath, _ := os.Executable()
	binary, e := os.ReadFile(binaryPath)
	h := sha256.Sum256(binary)
	proof, e2 := os.ReadFile("/out/kernel-verified.json")
	source := os.Getenv("BUTLER_SKILL_BROKER_SOURCE")
	if e != nil || e2 != nil || !strings.Contains(string(proof), hex.EncodeToString(h[:])) || len(source) != 40 {
		t.Fatal("bound kernel/artifact proof required before auth")
	}
	plan, e := os.ReadFile("/plan.json")
	expected, _ := json.MarshalIndent(skillBrokerPlan(), "", "  ")
	if e != nil || strings.TrimSpace(string(plan)) != string(expected) {
		t.Fatal("frozen plan changed")
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	control, e := pgx.Connect(ctx, "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable")
	if e != nil {
		t.Fatal("private control store unavailable")
	}
	var locked bool
	if control.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-laba-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		control.Close(ctx)
		t.Fatal("experimental auth source busy")
	}
	var others int
	if control.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()`).Scan(&others) != nil || others != 0 {
		control.Close(ctx)
		t.Fatal("another source client is active")
	}
	_, e = control.Exec(ctx, `CREATE SCHEMA skill_broker_diagnostic_v1; CREATE TABLE skill_broker_diagnostic_v1.window(trial text PRIMARY KEY,source text NOT NULL,plan jsonb NOT NULL,deadline timestamptz NOT NULL,finished_at timestamptz,result jsonb); CREATE TABLE skill_broker_diagnostic_v1.turns(seq integer PRIMARY KEY,dispatched boolean NOT NULL DEFAULT false,body jsonb)`)
	if e != nil {
		control.Close(ctx)
		t.Fatal("new one-use diagnostic schema required")
	}
	_, e = control.Exec(ctx, `INSERT INTO skill_broker_diagnostic_v1.window(trial,source,plan,deadline)VALUES($1,$2,$3,$4)`, skillBrokerTrial, source, plan, time.Now().Add(900*time.Second))
	if e != nil {
		control.Close(ctx)
		t.Fatal("one-use diagnostic unavailable")
	}
	s := &skillBrokerNative{control: control, source: source}
	t.Cleanup(func() { s.finish(t) })
	info, e := os.Lstat("/private/settings.key")
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 32 || info.Size() > 1024 {
		t.Fatal("private reference invalid")
	}
	key, e := os.ReadFile("/private/settings.key")
	if e != nil {
		t.Fatal("private reference unavailable")
	}
	defer clear(key)
	s.authPool, e = pgxpool.New(ctx, "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable")
	if e != nil {
		t.Fatal("private auth store unavailable")
	}
	s.manager = providerauth.NewManager(config.NewPostgresSettingsStore(s.authPool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	return s
}
func (s *skillBrokerNative) database(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	p, e := pgxpool.New(ctx, skillBrokerTarget)
	if e != nil {
		t.Fatal("new target store unavailable")
	}
	files, e := filepath.Glob("migrations/*.up.sql")
	if e != nil || len(files) == 0 {
		t.Fatal("migration source unavailable")
	}
	sort.Strings(files)
	for _, path := range files {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Exec(ctx, string(b)); e != nil {
			t.Fatalf("fresh target migration %s failed", filepath.Base(path))
		}
	}
	s.dbReady = true
	return p, skillBrokerTarget
}
func (s *skillBrokerNative) finish(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	if s.dbReady {
		pool, e := pgxpool.New(ctx, skillBrokerTarget)
		if e == nil {
			rows, e := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
			var tables []string
			if e == nil {
				for rows.Next() {
					var table string
					if rows.Scan(&table) == nil {
						tables = append(tables, table)
					}
				}
				rows.Close()
			}
			dump := map[string]json.RawMessage{}
			for _, table := range tables {
				var b []byte
				if pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM `+pgx.Identifier{table}.Sanitize()+` t`).Scan(&b) != nil {
					t.Error("target snapshot incomplete")
					break
				}
				dump[table] = b
			}
			if e = skillBrokerWrite("target-snapshot.json", dump); e != nil {
				t.Error("target snapshot retention failed")
			}
			pool.Close()
		} else {
			t.Error("target snapshot unavailable")
		}
	}
	known := 0
	unknown := 0
	native := 0
	for _, v := range s.turns {
		if v.Dispatched {
			native++
			if v.Usage == nil {
				unknown++
			} else {
				known += v.Usage.TotalTokens
			}
		}
	}
	result := map[string]any{"trial": skillBrokerTrial, "source": s.source, "turns": s.turns, "native_dispatched": native, "known_full_tokens": known, "unknown_usage": unknown, "pipeline_verified": s.verified && !t.Failed(), "invoice_cost": "unknown", "old_receipts": skillBrokerOldReceipts, "history_requalified": false, "production_changed": false, "owner_approvals": "scripted synthetic fixture only", "baseline_provider": "scripted proposals; real Broker and recipient"}
	b, _ := json.Marshal(result)
	if _, e := s.control.Exec(ctx, `UPDATE skill_broker_diagnostic_v1.window SET finished_at=clock_timestamp(),result=$1 WHERE trial=$2 AND finished_at IS NULL`, b, skillBrokerTrial); e != nil {
		t.Error("final durable receipt failed")
	}
	if e := skillBrokerWrite("result.json", result); e != nil {
		t.Error("final receipt retention failed")
	}
	if s.authPool != nil {
		s.authPool.Close()
	}
	_ = s.control.Close(ctx)
}

type skillBrokerObservedProvider struct {
	transport.ModelProvider
	state                        *skillBrokerNative
	digest                       string
	starts                       int
	sawDescription, sawProcedure bool
	runID                        string
}

func (s *skillBrokerNative) provider(t *testing.T, digest string) *skillBrokerObservedProvider {
	p, e := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 90 * time.Second, MaximumOutputTokens: 4096, AuthSource: s.manager})
	if e != nil {
		t.Fatal("native provider construction failed")
	}
	return &skillBrokerObservedProvider{ModelProvider: p, state: s, digest: digest}
}
func (p *skillBrokerObservedProvider) InferenceLimits() (int64, int64) { return 65536, 4096 }
func (p *skillBrokerObservedProvider) InferenceTimeout() time.Duration { return 90 * time.Second }
func (p *skillBrokerObservedProvider) close() {
	if p.runID != "" {
		_, _ = p.ModelProvider.CancelRun(context.Background(), transport.CancelRunRequest{RunID: p.runID, Reason: "durable fixture boundary owns continuation"})
	}
}
func (p *skillBrokerObservedProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	p.runID = r.Context.RunID
	for _, item := range r.InputItems {
		origin := content.ItemOrigin(item)
		for _, ref := range origin.SourceRefs {
			if strings.HasPrefix(ref, "skill-index:") {
				p.sawDescription = true
			}
			if ref == "skill:"+p.digest {
				if origin.IsInstruction() || item.Role != "user" {
					return nil, errors.New("procedure became authority")
				}
				p.sawProcedure = true
			}
		}
	}
	return p.capture(ctx, "start", r, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(c, r) })
}
func (p *skillBrokerObservedProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.capture(ctx, "tool_result", r, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.SubmitToolResult(c, r) })
}
func (p *skillBrokerObservedProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.capture(ctx, "continue", r, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(c, r) })
}
func (p *skillBrokerObservedProvider) capture(ctx context.Context, kind string, request any, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	s := p.state
	if s.failed || len(s.turns) >= 12 {
		return nil, errors.New("native diagnostic stopped or bounded")
	}
	entry := &skillBrokerTurn{Sequence: len(s.turns) + 1, Kind: kind, Request: request, Started: time.Now().UTC()}
	s.turns = append(s.turns, entry)
	if _, e := s.control.Exec(ctx, `INSERT INTO skill_broker_diagnostic_v1.turns(seq)VALUES($1)`, entry.Sequence); e != nil {
		s.failed = true
		return nil, errors.New("native attempt retention failed")
	}
	original := ctx
	guarded := transport.WithInferenceAuthority(ctx, func(c context.Context) error {
		if e := transport.CheckInferenceAuthority(original); e != nil {
			return e
		}
		var live bool
		e := s.control.QueryRow(c, `SELECT finished_at IS NULL AND deadline>clock_timestamp() FROM skill_broker_diagnostic_v1.window WHERE trial=$1`, skillBrokerTrial).Scan(&live)
		if e != nil || !live || s.failed {
			return errors.New("native diagnostic authority expired")
		}
		return nil
	})
	guarded = transport.WithInferenceDispatch(guarded, func(c context.Context) error {
		if e := transport.CheckInferenceDispatch(original); e != nil {
			return e
		}
		tag, e := s.control.Exec(c, `UPDATE skill_broker_diagnostic_v1.turns SET dispatched=true WHERE seq=$1 AND NOT dispatched`, entry.Sequence)
		if e != nil || tag.RowsAffected() != 1 {
			return errors.New("native diagnostic dispatch retention failed")
		}
		entry.Dispatched = true
		return nil
	})
	stream, err := call(guarded)
	var events []transport.TransportEvent
	if err != nil {
		entry.ErrorCode = "method_rejected"
	} else {
		for event := range stream {
			if event.Usage != nil {
				entry.Usage = event.Usage
			}
			if event.ProviderModelVersion != nil {
				entry.ModelVersion = event.ProviderModelVersion
			}
			if event.ToolCall != nil {
				entry.Proposals = append(entry.Proposals, *event.ToolCall)
			}
			if event.AssistantFinal != nil {
				entry.Final = event.AssistantFinal.Content
				if event.AssistantFinal.Usage != nil {
					entry.Usage = event.AssistantFinal.Usage
				}
			}
			if event.TransportError != nil {
				entry.ErrorCode = string(event.TransportError.Type)
			}
			events = append(events, event)
		}
	}
	entry.ElapsedMillis = time.Since(entry.Started).Milliseconds()
	if entry.ErrorCode != "" || entry.Usage == nil || !entry.Dispatched {
		s.failed = true
	}
	b, _ := json.Marshal(entry)
	persist, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if _, e := s.control.Exec(persist, `UPDATE skill_broker_diagnostic_v1.turns SET body=$2 WHERE seq=$1`, entry.Sequence, b); e != nil {
		s.failed = true
		return nil, errors.New("native usage retention failed")
	}
	if e := skillBrokerWrite(fmt.Sprintf("turn-%02d.json", entry.Sequence), entry); e != nil {
		s.failed = true
		return nil, errors.New("native receipt retention failed")
	}
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent, len(events))
	for _, event := range events {
		out <- event
	}
	close(out)
	return out, nil
}
func skillBrokerArtifact(raw json.RawMessage) string {
	var x any
	if json.Unmarshal(raw, &x) != nil {
		return ""
	}
	var walk func(any) string
	walk = func(x any) string {
		switch v := x.(type) {
		case map[string]any:
			if id, ok := v["artifact_id"].(string); ok && id != "" {
				return id
			}
			for _, child := range v {
				if id := walk(child); id != "" {
					return id
				}
			}
		case []any:
			for _, child := range v {
				if id := walk(child); id != "" {
					return id
				}
			}
		case string:
			var child any
			if json.Unmarshal([]byte(v), &child) == nil {
				return walk(child)
			}
		}
		return ""
	}
	return walk(x)
}
func (s *skillBrokerNative) verify(t *testing.T, pool *pgxpool.Pool, record task.Record, run, result, digest string, posts int32, site string) {
	t.Helper()
	ctx := context.Background()
	var pins int
	if pool.QueryRow(ctx, `SELECT count(*) FROM skill_usages WHERE run_id=$1 AND digest=$2 AND task_id=$3 AND task_revision=$4 AND constraints_revision=$5`, run, digest, record.TaskID, record.Revision, record.ConstraintsRevision).Scan(&pins) != nil || pins != 1 {
		t.Fatal("actual skill pin absent")
	}
	rows, e := pool.Query(ctx, `SELECT tool_name,status,result FROM tool_calls WHERE run_id=$1 ORDER BY started_at`, run)
	if e != nil {
		t.Fatal("tool receipts unavailable")
	}
	type receipt struct {
		Name, Status string
		Result       json.RawMessage
	}
	var receipts []receipt
	for rows.Next() {
		var r receipt
		if rows.Scan(&r.Name, &r.Status, &r.Result) != nil {
			rows.Close()
			t.Fatal("tool receipt invalid")
		}
		receipts = append(receipts, r)
	}
	rows.Close()
	loads := 0
	artifact := ""
	checked := ""
	for _, r := range receipts {
		if r.Status != "completed" {
			t.Fatal("noncompleted tool in native task")
		}
		switch r.Name {
		case "skills.load":
			loads++
		case "extension.project.prepare-patch":
			artifact = skillBrokerArtifact(r.Result)
		case "extension.project.check":
			checked = skillBrokerArtifact(r.Result)
		}
	}
	if loads != 1 || artifact == "" || checked != artifact || posts != 2 {
		t.Fatal("native load, unique patch or matching check absent")
	}
	get := func(path string) map[string]any {
		resp, e := http.Get(site + path)
		if e != nil {
			t.Fatal("recipient verification unavailable")
		}
		defer resp.Body.Close()
		var x map[string]any
		if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 32768)).Decode(&x) != nil {
			t.Fatal("recipient verification invalid")
		}
		return x
	}
	check := get("/project/patches/" + artifact + "/checks")
	if check["checks_passed"] != true || check["applied"] != false || check["artifact_id"] != artifact {
		t.Fatal("recipient objective check failed")
	}
	current := get("/project")
	body, _ := json.Marshal(current)
	if !strings.Contains(string(body), strings.TrimSpace(strings.Split(projectfixture.Original, "\n")[1])) {
		t.Fatal("original state changed")
	}
	proof := map[string]any{"task_id": record.TaskID, "run_id": run, "verified_result_id": result, "digest": digest, "actual_skill_pins": pins, "completed_skill_loads": loads, "recipient_artifact": artifact, "recipient_check": check, "current_project": current, "tool_receipts": receipts, "native_new_patch_posts": 1, "scripted_baseline_posts": 1, "owner_verification": "synthetic fixture owner"}
	if e = skillBrokerWrite("task-proof.json", proof); e != nil {
		t.Fatal("pipeline proof retention failed")
	}
	s.verified = true
	t.Log("NATIVE_SKILL_BROKER_PIPELINE_PASS exact real Broker load, current pinned DATA, durable approval/recovery, one checked unapplied artifact")
}
func TestSkillBrokerNativePlan(t *testing.T) {
	if os.Getenv("BUTLER_SKILL_BROKER_EXPORT") == "1" {
		if e := skillBrokerWrite("plan.json", skillBrokerPlan()); e != nil {
			t.Fatal(e)
		}
	}
}
func TestReviewedSkillNativeBrokerPipeline(t *testing.T) {
	if os.Getenv("BUTLER_SKILL_BROKER_NATIVE") != skillBrokerTrial {
		t.Skip("explicit isolated laba native pipeline only")
	}
	native := newSkillBrokerNative(t)
	runReviewedSkillRealBrokerE2E(t, native)
}
