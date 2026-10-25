package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

const behavior76Owner = "behavior76-synthetic-owner"

type behavior76Fixture struct {
	t            *testing.T
	ctx          context.Context
	pool         *pgxpool.Pool
	reads        *pgxpool.Pool
	sources      *catalog.Store
	tasks        *task.Store
	profiles     *profile.Store
	leases       *session.PostgresLeaseManager
	budgets      *budget.Controller
	controls     *task.Controller
	broker       *tools.BrokerClient
	manifestPath string
}

func newBehavior76Fixture(t *testing.T, parents ...context.Context) *behavior76Fixture {
	t.Helper()
	pool := memorytest.Database(t)
	u, err := url.Parse(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	db, err := postgresstore.OpenWithAuthorityReserve(context.Background(), postgresstore.Config{URL: u.String(), MaxConns: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	pool = db.Pool()
	parent := context.Background()
	if len(parents) > 0 {
		parent = parents[0]
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	t.Cleanup(cancel)
	path := filepath.Join(t.TempDir(), "deletions.log")
	manifest := catalog.NewFileManifest(path)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.NewWithRunObservationPool(pool, db.AuthorityReadPool(), manifest)
	leases := session.NewPostgresLeaseManagerWithAuthorityReads(pool, db.AuthorityReadPool())
	policy := budget.DefaultPolicy()
	policy.Models[antigravity.Name+":"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	quote := int64(1048576 + 8192)
	policy.OutputTokens = 8192
	policy.ModelMillis = 120000
	policy.ModelInputBytes = 131072
	policy.TaskLifetimeMillis = 1200000
	policy.Task.ModelTurns = 24
	policy.Task.Tokens = 24 * quote
	policy.Task.ExternalCalls = 36
	policy.Task.ActiveMillis = 24*policy.ModelMillis + 36*policy.ToolMillis
	policy.Mandate = policy.Task
	policy.Conversation = policy.Task
	policy.Day = policy.Task
	budgets := budget.NewControllerWithAuthorityReads(pool, db.AuthorityReadPool(), policy)
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	tasks := task.NewStoreWithAuthorityReads(pool, db.AuthorityReadPool())
	tasks.SetBudgets(budgets, "")
	f := &behavior76Fixture{t: t, ctx: ctx, pool: pool, reads: db.AuthorityReadPool(), sources: sources, tasks: tasks, profiles: profile.NewStore(pool), leases: leases, budgets: budgets, manifestPath: path}
	f.reconstruct()
	if _, err = pool.Exec(ctx, behavior76ObjectsSQL+`CREATE TABLE behavior76_aliases(kind text NOT NULL,key text NOT NULL,value text NOT NULL,PRIMARY KEY(kind,key)); CREATE TABLE behavior76_observations(n bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,observation jsonb NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	f.broker = f.startBroker()
	return f
}
func (f *behavior76Fixture) reconstruct() {
	f.tasks = task.NewStoreWithAuthorityReads(f.pool, f.reads)
	f.tasks.SetBudgets(f.budgets, "")
	f.profiles = profile.NewStore(f.pool)
	f.sources = catalog.NewWithRunObservationPool(f.pool, f.reads, catalog.NewFileManifest(f.manifestPath))
	f.leases = session.NewPostgresLeaseManagerWithAuthorityReads(f.pool, f.reads)
	f.controls = task.NewController(f.tasks, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), input.NewStore(f.pool), transcript.NewStore(f.pool))
}
func (f *behavior76Fixture) actor(delivery string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: behavior76Owner, Channel: "web", DeliveryID: delivery})
}
func (f *behavior76Fixture) alias(kind, key, value string) {
	f.t.Helper()
	_, err := f.pool.Exec(f.ctx, `INSERT INTO behavior76_aliases(kind,key,value) VALUES($1,$2,$3) ON CONFLICT(kind,key) DO UPDATE SET value=excluded.value`, kind, key, value)
	if err != nil {
		f.t.Fatal(err)
	}
}
func (f *behavior76Fixture) lookup(kind, key string) string {
	f.t.Helper()
	var v string
	if err := f.pool.QueryRow(f.ctx, `SELECT value FROM behavior76_aliases WHERE kind=$1 AND key=$2`, kind, key).Scan(&v); err != nil {
		f.t.Fatal("fixture alias unavailable", kind, key, err)
	}
	return v
}

func (f *behavior76Fixture) startBroker() *tools.BrokerClient {
	t := f.t
	t.Helper()
	binary := os.Getenv("BUTLER_BEHAVIOR76_BROKER_BINARY")
	if binary == "" {
		t.Fatal("explicit compiled production Broker required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(server, &behavior76Runtime{pool: f.pool})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	registry := map[string]any{"tools": []any{
		map[string]any{"tool_name": "http.request", "tool_class": "http", "description": "Declared synthetic test object recipient. GET https://fixture.invalid/objects/KEY observes actual body/version/sha256/prepared_body/committed_body/commit_count. POST same URL commits with body containing JSON {expected_version,expected_sha256,body}. CAS is mandatory. No real network recipient.", "status": "active", "runtime_target": listener.Addr().String(), "contract_revision": "behavior76-test-v1", "input_schema_json": `{"type":"object","additionalProperties":false,"required":["method","url"],"properties":{"method":{"type":"string","enum":["GET","POST"]},"url":{"type":"string"},"body":{"type":"string"}}}`, "output_schema_json": `{"type":"object"}`, "effect_contract": map[string]any{"retry_safety": "external_idempotency", "verification_method": "external_receipt_v1", "idempotency_header": "Idempotency-Key", "scope_origin": "https://fixture.invalid"}},
		map[string]any{"tool_name": "doctor.generate_report", "tool_class": "doctor", "description": "Declared synthetic local artifact preparation only: record prepared_body on the owned test object, preserving actual body/committed_body/count. Input is current resource_key, expected_version, expected_sha256 and desired body. No publish/send/network. Genuine Broker prepare-class authorization is required.", "status": "active", "runtime_target": listener.Addr().String(), "contract_revision": "behavior76-test-v1", "input_schema_json": `{"type":"object","additionalProperties":false,"required":["resource_key","expected_version","expected_sha256","body"],"properties":{"resource_key":{"type":"string"},"expected_version":{"type":"integer"},"expected_sha256":{"type":"string"},"body":{"type":"string"}}}`, "output_schema_json": `{"type":"object"}`, "effect_contract": map[string]any{"retry_safety": "unsafe", "verification_method": "local_runtime_ack"}},
	}}
	// Fixed operator-owned test recipient disclosure scope. This registry field
	// is part of the real contract hash, never selected by a model response.
	registry["tools"].([]any)[0].(map[string]any)["allowed_domains"] = []string{"fixture.invalid"}
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "tools.json")
	tokenPath := filepath.Join(dir, "token")
	const token = "behavior76-private-synthetic-service-token"
	if err = os.WriteFile(registryPath, []byte(behavior76JSON(registry)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	approvalsService := approvals.NewService(approvals.NewPostgresRepository(f.pool), nil)
	reader := behavior76ContextReader{&actionContextReader{runs: run.NewService(run.NewPostgresRepository(f.pool), nil), sessions: session.NewPostgresRepository(f.pool), leases: f.leases, tasks: f.tasks, approvals: approvalsService, pool: f.pool}}
	control := httptest.NewServer(actionauth.NewContextHandler(token, reader))
	t.Cleanup(control.Close)
	dsn, err := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	address := freeAddress(t)
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=4", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+control.URL, "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath, "BUTLER_EXECUTION_BUDGET_POLICY_JSON="+behavior76JSON(f.budgets.Policy))
	log, err := os.OpenFile(filepath.Join(dir, "broker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = log.Close() })
	client, err := tools.DialWithTokenFile(address, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		if _, err = client.ListTools(f.ctx); err == nil {
			return client
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("separate production Broker failed to start", err)
	return nil
}

func (f *behavior76Fixture) makeCore(provider transport.ModelProvider, mandate string) *flow.Service {
	return f.makeCoreWithAssembler(provider, mandate, prompt.NewAssembler())
}
func (f *behavior76Fixture) makeCoreWithAssembler(provider transport.ModelProvider, mandate string, assembler flow.PromptAssembler) *flow.Service {
	results := completion.New(f.pool)
	results.DataGuard = f.sources.CheckRunSourcesTx
	return flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{
		ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel, OwnerID: "behavior76-worker", LeaseTTL: 60,
		AutonomyMode: 3, ActionMandateID: mandate, Tasks: f.tasks, Checkpoints: input.NewStore(f.pool), Budgets: f.budgets, Completion: results,
		MemorySources: f.sources, Tools: f.broker, ToolCatalog: f.broker, ActionAuthorizer: discoveryBrokerDiagnostics{BrokerClient: f.broker, t: f.t},
		ProfileStore: profileStoreAdapter{store: f.profiles, sources: f.sources}, PromptManager: prompt.NewStaticManager(), PromptAssembler: assembler,
		SemanticSearchEnabled: true, SemanticMemory: behavior76Documents{f: f},
		OwnerGoals: goal.New(f.pool, f.tasks, f.controls),
	}, nil)
}

func behavior76Data(v any, refs ...string) transport.InputItem {
	return content.Data(v, content.Origin{Kind: content.ToolOutput, SourceRefs: refs, Sensitivity: content.Private})
}

func (f *behavior76Fixture) seed(e behavior76Episode) {
	for _, v := range e.Initial.Tasks {
		sessionKey := "behavior76:" + e.ID + ":" + v.Key
		parent := ""
		if v.ParentTaskKey != "" {
			parent = f.lookup("task", v.ParentTaskKey)
			sessionKey = f.lookup("session", v.ParentTaskKey)
		}
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web') ON CONFLICT(session_key) DO NOTHING`, sessionKey, behavior76Owner); err != nil {
			f.t.Fatal(err)
		}
		r, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: sessionKey, OwnerID: behavior76Owner, ParentTaskID: parent, Goal: v.Goal, Constraints: v.Constraints, CompletionCriteria: v.CompletionCriteria, InitialExecutionPolicy: v.ExecutionPolicy, Source: map[string]any{"type": "synthetic_behavior76", "episode": e.ID, "task_key": v.Key}})
		if err != nil {
			f.t.Fatal(err)
		}
		f.alias("task", v.Key, r.TaskID)
		f.alias("session", v.Key, sessionKey)
		if _, err = f.tasks.SaveWorking(f.ctx, r.TaskID, r.WorkingRevision, v.Working); err != nil {
			f.t.Fatal(err)
		}
		if v.Status != "" && v.Status != "active" {
			if _, err = f.tasks.Transition(f.actor("fixture-initial-status"), r.TaskID, r.Revision, v.Status, ""); err != nil {
				f.t.Fatal("unsupported initial task state", err)
			}
		}
	}
	for _, v := range e.Initial.Resources {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO behavior76_objects(key,task_key,task_id,body,version,sha256) VALUES($1,$2,$3,$4,$5,$6)`, v.Key, v.TaskKey, f.lookup("task", v.TaskKey), v.Body, v.Version, behavior76Hash([]byte(v.Body))); err != nil {
			f.t.Fatal(err)
		}
	}
	for _, v := range e.Initial.Profiles {
		f.seedProfile(v)
	}
	for _, v := range e.Initial.Documents {
		src, err := f.sources.Put(f.actor("document-initial"), catalog.PutSource{ExternalKey: v.SourceKey, Kind: "document", ScopeType: "user", ScopeID: behavior76Owner, Title: v.Title, Body: v.Body})
		if err != nil {
			f.t.Fatal(err)
		}
		f.alias("source", v.SourceKey, src.ID)
		f.alias("document", v.SourceKey, src.ID)
	}
	for _, v := range e.Initial.PriorObservations {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO behavior76_observations(observation) VALUES($1)`, behavior76JSON(v)); err != nil {
			f.t.Fatal(err)
		}
	}
}
func (f *behavior76Fixture) seedProfile(v behavior76Profile) {
	scope := v.ScopeID
	if v.ScopeType == "user" && scope == "owner" {
		scope = behavior76Owner
	} else if v.ScopeType == "session" {
		scope = f.lookup("session", scope)
	}
	src, err := f.sources.Put(f.actor("profile-initial"), catalog.PutSource{ExternalKey: v.SourceKey, Kind: "document", ScopeType: v.ScopeType, ScopeID: scope, Title: "Synthetic preference source", Body: v.Summary})
	if err != nil {
		f.t.Fatal(err)
	}
	f.alias("source", v.SourceKey, src.ID)
	value, _ := json.Marshal(v.Summary)
	claim, err := f.sources.Propose(f.actor("profile-propose"), catalog.ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: v.Key, Value: value, Summary: v.Summary, Confidence: 1})
	if err != nil {
		f.t.Fatal(err)
	}
	f.alias("claim", v.SourceKey, claim.ID)
	if v.Status == "accepted" || v.Status == "revoked" {
		claim, err = f.sources.Decide(f.actor("profile-accept"), catalog.Decision{ClaimID: claim.ID})
		if err != nil {
			f.t.Fatal(err)
		}
		if v.Status == "revoked" {
			if _, err = f.sources.Delete(f.actor("profile-revoke"), src.ID, "revoke", src.Revision); err != nil {
				f.t.Fatal(err)
			}
		}
	}
}

func (f *behavior76Fixture) mandate(taskKey string) string {
	r, err := f.tasks.Get(f.ctx, f.lookup("task", taskKey))
	if err != nil {
		f.t.Fatal(err)
	}
	id := fmt.Sprintf("behavior76:%s:r%d", r.TaskID, r.Revision)
	var exists bool
	if err = f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM behavior76_task_mandates WHERE task_id=$1 AND task_revision=$2)`, r.TaskID, r.Revision).Scan(&exists); err != nil {
		f.t.Fatal(err)
	}
	if exists {
		return id
	}
	// The existing task mandate API accepts origins, not arbitrary URL paths.
	// The test recipient additionally enforces exact object→task ownership/CAS.
	resources := []string{"tool:doctor.generate_report", "https://fixture.invalid"}
	m := actionauth.Mandate{ID: id, OwnerID: behavior76Owner, TaskID: r.TaskID, TaskRevision: r.Revision, ConstraintsRevision: r.ConstraintsRevision, Revision: 1, Tools: []string{"http.request", "doctor.generate_report"}, Resources: resources, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare, actionauth.EffectCommit}, ExpiresAt: time.Now().Add(20 * time.Minute)}
	if err = actionauth.NewPostgresStore(f.pool).PutMandate(f.actor("fixed-test-policy"), m); err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO behavior76_task_mandates VALUES($1,$2,$3)`, r.TaskID, r.Revision, id); err != nil {
		f.t.Fatal(err)
	}
	return id
}
