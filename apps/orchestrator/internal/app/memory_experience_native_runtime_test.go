package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/readcontrol"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const experienceNativeOwner = "owner" // same actual owner/day lineage as 84/85/88/89
const experienceNativePreference = "Предпочтение владельца для текущего поручения: отвечать по-русски."

type experienceNativeAddresses struct {
	Extension, HTTP, Broker, BrokerHTTP string
	reserved                            *experienceNativePortReservations
}

// Keep the frozen endpoints owned while the rest of the fixture allocates ports.
// The child handoff releases only that child's sockets immediately before Start;
// stopping it reacquires those exact endpoints before any later fixture setup.
type experienceNativePortReservations struct {
	mu        sync.Mutex
	listeners map[string]net.Listener
	closed    bool
}

func (p *experienceNativePortReservations) reclaim(addresses ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	for _, address := range addresses {
		if _, ok := p.listeners[address]; !ok {
			return fmt.Errorf("endpoint was not frozen: %s", address)
		}
		if p.listeners[address] != nil {
			continue
		}
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return fmt.Errorf("reclaim frozen endpoint %s: %w", address, err)
		}
		p.listeners[address] = listener
	}
	return nil
}

func (p *experienceNativePortReservations) handoff(start func() error, addresses ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("frozen endpoint reservations already closed")
	}
	for _, address := range addresses {
		if listener, ok := p.listeners[address]; !ok || listener == nil {
			return fmt.Errorf("frozen endpoint %s is not reserved", address)
		}
	}
	for _, address := range addresses {
		if err := p.listeners[address].Close(); err != nil {
			return err
		}
		p.listeners[address] = nil
	}
	return start()
}

func (p *experienceNativePortReservations) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for address, listener := range p.listeners {
		if listener != nil {
			_ = listener.Close()
			p.listeners[address] = nil
		}
	}
}

// Freeze these before original work and reuse the exact endpoints in every
// bounded subtest. Random per-case runtime ports would change contract hashes
// and turn an environment/currentness comparison into a different experiment.
func newExperienceNativeAddresses(t *testing.T) experienceNativeAddresses {
	t.Helper()
	p := &experienceNativePortReservations{listeners: make(map[string]net.Listener)}
	t.Cleanup(p.close)
	allocate := func() string {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal("reserve frozen runtime endpoint", err)
		}
		address := listener.Addr().String()
		p.listeners[address] = listener
		return address
	}
	return experienceNativeAddresses{Extension: allocate(), HTTP: allocate(), Broker: allocate(), BrokerHTTP: allocate(), reserved: p}
}

func experienceNativeChild(t *testing.T, reservations *experienceNativePortReservations, name, directory, binary, grpcAddress, httpAddress string, args []string, environment ...string) func() {
	t.Helper()
	logPath := filepath.Join(directory, name+".log")
	if proof := os.Getenv("BUTLER_EXPERIENCE_PROCESS_PROOF"); filepath.IsAbs(proof) {
		if err := os.MkdirAll(proof, 0700); err != nil {
			t.Fatal("private component evidence unavailable")
		}
		logPath = filepath.Join(proof, experienceDigest(directory)+"-"+name+".log")
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("private component log already used/unavailable")
	}
	cmd := exec.Command(binary, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR=" + grpcAddress, "BUTLER_HTTP_ADDR=" + httpAddress}, environment...)
	cmd.Stdout, cmd.Stderr = file, file
	addresses := []string{grpcAddress}
	if httpAddress != "127.0.0.1:0" {
		addresses = append(addresses, httpAddress)
	}
	if err = reservations.handoff(cmd.Start, addresses...); err != nil {
		file.Close()
		t.Fatal("isolated actual component start failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			file.Close()
			if err := reservations.reclaim(addresses...); err != nil {
				t.Error("frozen child endpoint reservation lost", err)
			}
		})
	}
	t.Cleanup(stop)
	for until := time.Now().Add(10 * time.Second); time.Now().Before(until); {
		connection, err := net.DialTimeout("tcp", grpcAddress, 100*time.Millisecond)
		if err == nil {
			connection.Close()
			return stop
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("actual component did not become ready")
	return stop
}

type experienceNativeToolCatalog struct {
	client *tools.BrokerClient
	timing *experienceSourceTimings
}

func (c *experienceNativeToolCatalog) ListToolsForRun(ctx context.Context, run string) ([]*toolbrokerv1.ToolContract, error) {
	if c.client == nil {
		return nil, fmt.Errorf("current actual Broker not ready")
	}
	start := time.Now()
	contracts, err := c.client.ListToolsForRun(ctx, run)
	phase := "other_tools_rpc"
	if checked, _ := ctx.Value(experienceSourceTimingKey{}).(bool); checked {
		phase = "source_tools_rpc"
	}
	c.timing.record(ctx, phase, start, err)
	return contracts, err
}

type experienceNativeRuntime struct {
	pool        *pgxpool.Pool
	cat         *catalog.Store
	lessons     *experience.Store
	broker      *tools.BrokerClient
	skillClient *skills.Client
	goals       *goal.Manager
	tasks       *task.Store
	sessions    *session.PostgresRepository
	runs        *runservice.Service
	history     *transcript.Store
	inputs      *durableinput.Store
	results     *completion.Coordinator
	journal     *actionauth.PostgresStore
	budgets     *budget.Controller
	pm          *promptmgmt.Manager
	decisions   *approvals.Service
	extensions  *extensions.ControlClient
	pack        extensions.Package
	generation  int64
	environment string
	expires     time.Time
	addresses   experienceNativeAddresses
	timing      *experienceSourceTimings
	stops       []func()
	reconcile   func(context.Context, string, string, string) (actionauth.RunEffect, error)
	foreground  *experienceNativeForeground
}

// Explicit test-only composition. Nil retains the frozen90 foreground defaults;
// a later coherent pilot declares its own context and requests the real temporal
// predicate. This never changes provider choice, source guards or qualification.
type experienceNativeForeground struct {
	ContextTokens   int
	TemporalEnabled bool
}

func experienceNativeForegroundConfig(cfg flow.Config, result providerfactory.Result, controller *budget.Controller, selected *experienceNativeForeground) flow.Config {
	if selected != nil {
		cfg.ContextBudgetTokens = selected.ContextTokens
		adapter := cfg.ProfileStore.(profileStoreAdapter)
		adapter.temporal = temporalReadConfigured(selected.TemporalEnabled, result, controller)
		cfg.ProfileStore = adapter
	}
	return cfg
}

// Stop every actual client/runtime before freezing a PostgreSQL template or
// reusing the fixed endpoint set. Cleanup is idempotent and never deletes data.
func (r *experienceNativeRuntime) close() {
	for i := len(r.stops) - 1; i >= 0; i-- {
		r.stops[i]()
	}
	r.stops = nil
}

// Only test composition differs: the recipient and still-unqualified experience
// reader are constructor-injected. Tools, current private context, grant checks,
// runtime routing, effect journal, owned completion and skills are production.
func newExperienceNativeRuntime(t *testing.T, pool *pgxpool.Pool, cat *catalog.Store, anchor, directory string, recipient *experienceRecipient, addresses experienceNativeAddresses, policy budget.Policy, prompt, arm string, subset []catalog.SourceRef, expires time.Time) *experienceNativeRuntime {
	return newExperienceNativeRuntimeConfigured(t, pool, cat, anchor, directory, recipient, addresses, policy, prompt, arm, subset, expires, nil)
}

func newExperienceNativeRuntimeConfigured(t *testing.T, pool *pgxpool.Pool, cat *catalog.Store, anchor, directory string, recipient *experienceRecipient, addresses experienceNativeAddresses, policy budget.Policy, prompt, arm string, subset []catalog.SourceRef, expires time.Time, foreground *experienceNativeForeground) *experienceNativeRuntime {
	t.Helper()
	if foreground != nil && (foreground.ContextTokens <= 4096 || foreground.ContextTokens > 1048576) {
		t.Fatal("finite explicit foreground context required")
	}
	if addresses.reserved == nil {
		t.Fatal("runtime endpoints must be frozen and reserved before fixture setup")
	}
	if err := addresses.reserved.reclaim(addresses.Extension, addresses.HTTP, addresses.Broker, addresses.BrokerHTTP); err != nil {
		t.Fatal("frozen runtime endpoints unavailable before fixture setup", err)
	}
	ctx := context.Background()
	r := &experienceNativeRuntime{pool: pool, cat: cat, tasks: task.NewStore(pool), sessions: session.NewPostgresRepository(pool), runs: runservice.NewService(runservice.NewPostgresRepository(pool), nil), history: transcript.NewStore(pool), inputs: durableinput.NewStore(pool), results: completion.New(pool), journal: actionauth.NewPostgresStore(pool), budgets: budget.NewController(pool, policy), expires: expires, addresses: addresses}
	if foreground != nil {
		copy := *foreground
		r.foreground = &copy
	}
	r.goals = goal.New(pool, r.tasks, nil)
	r.timing = newExperienceSourceTimings(t, r, directory)
	r.budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	leasing := session.NewPostgresLeaseManager(pool)
	approvalRepo := approvals.NewPostgresRepository(pool)
	approvalRepo.SetCheckpointHooks(r.inputs)
	r.decisions = approvals.NewService(approvalRepo, nil)
	currentTools := &experienceNativeToolCatalog{timing: r.timing}
	artifactPins := []string{experienceEnvironment(recipient.snapshot().Config)}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("compiled recipient identity unavailable")
	}
	compiledRecipient, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal("compiled recipient artifact unavailable")
	}
	artifactPins = append(artifactPins, experienceDigest(string(compiledRecipient)))
	for _, key := range []string{"BUTLER_BROKER_E2E_BINARY", "BUTLER_EXTENSION_E2E_BINARY", "BUTLER_EXPERIENCE_HTTP_BINARY"} {
		raw, err := os.ReadFile(os.Getenv(key))
		if err != nil {
			t.Fatal("frozen actual runtime artifact missing", key)
		}
		artifactPins = append(artifactPins, experienceDigest(string(raw)))
	}
	r.environment = experienceDigest(strings.Join(artifactPins, "\x00"))
	r.lessons, err = experience.New(cat, r.environment, currentTools)
	if err != nil {
		t.Fatal("operator-pinned experience environment unavailable", err)
	}
	r.results.Evidence = effectEvidence{journal: r.journal}
	r.results.DataGuard = func(c context.Context, tx pgx.Tx, run string) error {
		if err := cat.CheckRunSourcesTx(c, tx, run); err != nil {
			return err
		}
		if err := r.lessons.CheckCapturedPinsTx(c, tx, run); err != nil {
			return err
		}
		return skills.CheckPinnedLifecycleTx(c, tx, run)
	}
	coreToken, runtimeToken := strings.Repeat("experience-private-core-", 3), strings.Repeat("experience-private-runtime-", 3)
	file := func(name string, value []byte) string {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal("private owning runtime file unavailable")
		}
		return path
	}
	coreFile, runtimeFile := file("core.token", []byte(coreToken)), file("runtime.token", []byte(runtimeToken))
	reader := &actionContextReader{runs: r.runs, sessions: r.sessions, leases: leasing, tasks: r.tasks, approvals: r.decisions, pool: pool, memoryReadReady: arm != experienceNativeArms[0], lessons: r.lessons}
	private := http.NewServeMux()
	if reader.memoryReadReady {
		reads, err := NewMemoryReads(pool, cat, AdaptiveReadOptions{Experience: r.lessons, sourceSubset: subset})
		if err != nil {
			t.Fatal(err)
		}
		handler, err := readcontrol.NewHandler(coreToken, reads)
		if err != nil {
			t.Fatal(err)
		}
		private.Handle(readcontrol.Path, handler)
	}
	verifier := &skillEvidence{tasks: r.tasks, journal: r.journal, transcript: r.history, budgets: budget.NewLedger(pool), results: r.results, sources: cat, lessons: r.lessons}
	private.Handle("/internal/skill-evidence", skills.NewEvidenceHandler(coreToken, verifier))
	private.Handle("/", actionauth.NewContextHandler(coreToken, reader))
	authority, site := httptest.NewServer(private), httptest.NewServer(recipient)
	r.stops = append(r.stops, authority.Close, site.Close)
	t.Cleanup(r.close)
	extBinary := os.Getenv("BUTLER_EXTENSION_E2E_BINARY")
	binary, err := os.ReadFile(extBinary)
	if err != nil {
		t.Fatal("frozen compiled extension runtime unavailable")
	}
	r.pack, err = projectfixture.Manifest(experienceDigest(string(binary)), "experience-native-recipient-"+recipient.snapshot().Config.Version)
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := json.Marshal(map[string]extensions.Profile{"reference-http": {Target: addresses.Extension, Dependency: r.pack.Manifest.Dependencies[0]}})
	remaining := min(time.Until(expires).Milliseconds(), int64((40 * time.Minute).Milliseconds()))
	if remaining < 1 {
		t.Fatal("earlier independent window deadline expired")
	}
	r.stops = append(r.stops, experienceNativeChild(t, addresses.reserved, "extension", directory, extBinary, addresses.Extension, "127.0.0.1:0", []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_EGRESS_HARNESS_LIFETIME_MS="+fmt.Sprint(remaining), "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile))
	r.stops = append(r.stops, experienceNativeChild(t, addresses.reserved, "http", directory, os.Getenv("BUTLER_EXPERIENCE_HTTP_BINARY"), addresses.HTTP, "127.0.0.1:0", []string{"-test.run=^TestExperienceHTTPRuntimeHarness$"}, "BUTLER_EXPERIENCE_HTTP_HARNESS=1", "BUTLER_EXPERIENCE_HTTP_HARNESS_LIFETIME_MS="+fmt.Sprint(remaining), "BUTLER_EXPERIENCE_RECIPIENT_ENDPOINT="+site.URL))
	registry, err := os.ReadFile("../../../../configs/tools.json")
	if err != nil {
		t.Fatal("actual HTTP contract unavailable")
	}
	var all struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(registry, &all) != nil {
		t.Fatal("actual registry unreadable")
	}
	selected := []map[string]any{}
	for _, contract := range all.Tools {
		if contract["tool_name"] == "http.request" {
			contract["runtime_target"], contract["allowed_domains"] = addresses.HTTP, []string{"external.test"}
			selected = append(selected, contract)
		}
	}
	if len(selected) != 1 {
		t.Fatal("exact HTTP verifier contract required")
	}
	raw, _ := json.Marshal(map[string]any{"tools": selected})
	registryFile := file("registry.json", raw)
	dsn, err := url.Parse(pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	policyJSON, _ := json.Marshal(policy)
	env := []string{"BUTLER_POSTGRES_URL=" + dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_EXECUTION_BUDGET_POLICY_JSON=" + string(policyJSON), "BUTLER_TOOL_REGISTRY_PATH=" + registryFile, "BUTLER_ACTION_CONTEXT_URL=" + authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE=" + coreFile, "BUTLER_EXTENSION_PROFILES_JSON=" + string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE=" + runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID=" + experienceNativeOwner, "BUTLER_MEMORY_DELETION_MANIFEST_FILE=" + anchor}
	if reader.memoryReadReady {
		env = append(env, "BUTLER_MEMORY_READ_ENABLED=true", "BUTLER_MEMORY_READ_CONTROL_URL="+authority.URL+readcontrol.Path)
	}
	if arm == experienceNativeArms[2] {
		env = append(env, "BUTLER_SKILLS_ENABLED=true")
	}
	r.stops = append(r.stops, experienceNativeChild(t, addresses.reserved, "broker", directory, os.Getenv("BUTLER_BROKER_E2E_BINARY"), addresses.Broker, addresses.BrokerHTTP, nil, env...))
	r.broker, err = tools.DialWithTokenFile(addresses.Broker, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	r.stops = append(r.stops, func() { _ = r.broker.Close() })
	currentTools.client = r.broker
	r.extensions = extensions.NewControlClient("http://"+addresses.BrokerHTTP+"/internal/extensions", coreToken)
	// The owning operator endpoint is deliberately absent from the model registry.
	// Only an exact independently captured Broker read result can reconcile it.
	controlTransport := &http.Transport{Proxy: nil}
	r.stops = append(r.stops, controlTransport.CloseIdleConnections)
	controlClient := &http.Client{Transport: controlTransport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r.reconcile = func(ctx context.Context, action, authorization, result string) (actionauth.RunEffect, error) {
		var out actionauth.RunEffect
		body, err := json.Marshal(map[string]string{"action_id": action, "observation_authorization_id": authorization, "result_json": result})
		if err != nil {
			return out, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addresses.BrokerHTTP+"/internal/action-effects", bytes.NewReader(body))
		if err != nil {
			return out, err
		}
		request.Header.Set("Authorization", "Bearer "+coreToken)
		response, err := controlClient.Do(request)
		if err != nil {
			return out, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return out, fmt.Errorf("independent reconciliation denied: HTTP %d", response.StatusCode)
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&out)
		return out, err
	}
	delivery := experienceDigest(directory)
	control := func(q extensions.ControlRequest) extensions.ControlResponse {
		t.Helper()
		out, err := r.extensions.Control(experienceNativeActor(ctx, delivery+":"+q.Operation), q)
		if err != nil || out.Error != "" {
			t.Fatal("actual reviewed extension preparation failed", q.Operation, err)
		}
		return out
	}
	names := []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	existing := control(extensions.ControlRequest{Operation: "list"})
	for _, item := range existing.Items {
		if item.Record.ID == "project" && item.Record.ManifestHash == r.pack.Digest && item.Record.Available {
			r.generation = item.Record.Generation
		}
	}
	if r.generation == 0 {
		control(extensions.ControlRequest{Operation: "stage", ManifestJSON: r.pack.Raw})
		control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: r.pack.Digest})
		state := control(extensions.ControlRequest{Operation: "list"}).Items[0].Record
		state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: r.pack.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: names}).Item.Record
		control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
		state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
		r.generation = state.Generation
	}
	if arm == experienceNativeArms[2] {
		r.skillClient = skills.NewClient("http://"+addresses.BrokerHTTP+"/internal/skills", coreToken)
	}
	r.pm = promptmgmt.NewManager(config.NewPostgresSettingsStore(pool))
	on := true
	if _, err = r.pm.Update(ctx, promptmgmt.UpdateRequest{BasePrompt: prompt, Enabled: &on, UpdatedBy: "experience-native-frozen"}); err != nil {
		t.Fatal("common current prompt unavailable")
	}
	return r
}

func experienceNativeActor(ctx context.Context, delivery string) context.Context {
	return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: experienceNativeOwner, Channel: "web", DeliveryID: delivery})
}

func (r *experienceNativeRuntime) acceptCurrentProfile(t *testing.T, ctx context.Context, record task.Record) {
	t.Helper()
	owner := experienceNativeActor(ctx, "current-language:"+record.TaskID)
	src, err := r.cat.Put(owner, catalog.PutSource{ExternalKey: "current-language:" + record.TaskID, Kind: "document", ScopeType: "task", ScopeID: record.TaskID, Title: "Current owner language preference", Body: experienceNativePreference})
	if err != nil {
		t.Fatal("current owner preference source", err)
	}
	value, _ := json.Marshal("ru")
	c, err := r.cat.Propose(owner, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "preference.answer_language", Value: value, Summary: experienceNativePreference, Confidence: 1})
	if err != nil {
		t.Fatal("current owner preference proposal", err)
	}
	if _, err = r.cat.Decide(owner, catalog.Decision{ClaimID: c.ID}); err != nil {
		t.Fatal("explicit owner preference acceptance", err)
	}
}

func (r *experienceNativeRuntime) createTask(t *testing.T, ctx context.Context, id, instruction, resource, permissions string) task.Record {
	t.Helper()
	names, err := experienceNativePermissionTools(permissions)
	if err != nil {
		t.Fatal("current frozen permission profile rejected")
	}
	key := "web:" + id
	if _, _, err := r.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: experienceNativeOwner, Channel: "web", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	g, err := r.goals.PutGoal(experienceNativeActor(ctx, id+":goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: instruction, Purpose: "experience_current_task", ExpiresAt: r.expires}})
	if err != nil {
		t.Fatal("current owner goal admission", err)
	}
	allowed := append(append([]string{}, names...), "http.request", readcontrol.Search, readcontrol.Open, readcontrol.Neighbors, "skills.search", "skills.load")
	effects := []actionauth.Effect{actionauth.EffectRead}
	if len(names) == 3 {
		effects = append(effects, actionauth.EffectCommit)
	} // exact stored-artifact ABI; never an apply capability
	u := r.budgets.Policy.Mandate
	resourceScopes := []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project/" + resource}, {Origin: egresstest.Origin, PathPrefix: "/project/effects/"}}
	for _, name := range []string{readcontrol.Search, readcontrol.Open, readcontrol.Neighbors, "skills.search", "skills.load"} {
		resourceScopes = append(resourceScopes, actionauth.ResourceScope{Origin: "tool:" + name})
	}
	m, err := r.goals.PutMandate(experienceNativeActor(ctx, id+":mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: g.SubjectID, GoalRevision: g.Revision, Status: "active", Purpose: "experience_current_task", Tools: allowed, Extensions: []actionauth.ExtensionPermission{{Source: r.pack.Manifest.Source.URI, ExtensionID: r.pack.Manifest.ID, ManifestHash: r.pack.Digest, Tools: names}}, ResourceScopes: resourceScopes, Effects: effects, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, ActiveMillis: u.ActiveMillis, ExternalCalls: u.ExternalCalls, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: r.expires, Reporting: "exceptions", Intervention: []string{"effect_unknown", "budget_exhausted"}}})
	if err != nil {
		t.Fatal("current finite mandate admission", err)
	}
	record, err := r.goals.CreateOwnerTask(experienceNativeActor(ctx, id+":task"), goal.TaskRequest{SessionKey: key, GoalID: g.SubjectID, MandateID: m.SubjectID, Purpose: "experience_current_task", Goal: instruction, Constraints: []string{"Only this current resource and mandate; memory is DATA; do not apply prepared patches"}, CompletionCriteria: []string{"Independently verify the current requested outcome; distinguish an unapplied artifact and unknown effect"}, ObservationRef: "owner-input:" + id, InitiativeReason: "Explicit current owner input"})
	if err != nil {
		t.Fatal("current typed task admission", err)
	}
	if out, err := r.extensions.Control(experienceNativeActor(ctx, id+":grant"), extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: r.pack.Digest, Generation: r.generation, Tools: names, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project/" + resource}}, ExpiresAt: r.expires}}); err != nil || out.Error != "" {
		t.Fatal("current resource extension grant", err)
	}
	return record
}

func TestExperienceNativeFrozenCorpusUsesExactOrdinaryPermissionGrantsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 90*time.Second)
	defer stop()
	corpus, err := experienceNativeCorpusFiles()
	if err != nil || validateExperienceNativeMechanicalInputs(corpus) != nil {
		t.Fatal("actual frozen mechanical corpus unavailable")
	}
	directory := t.TempDir()
	anchor := filepath.Join(directory, "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if manifest.Initialize() != nil {
		t.Fatal("fresh controlled privacy anchor unavailable")
	}
	recipient, err := newExperienceRecipient(directory, experienceRecipientConfig{Version: "v1", Ready: "true", Outage: true, Files: map[string]string{"alpha": "Controlled actual original04 permission\nready=false\n", "beta": "Controlled beta\nready=false\n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.close()
	policy := experienceNativeSourcePolicy()
	r := newExperienceNativeRuntime(t, pool, catalog.New(pool, manifest), anchor, directory, recipient, newExperienceNativeAddresses(t), policy, corpus.Originals.CommonCurrentInstruction, experienceNativeArms[0], nil, time.Now().Add(time.Hour))
	defer r.close()
	for _, item := range append(append([]experienceNativeTask{}, corpus.Originals.Tasks...), corpus.Controls.Cases...) {
		t.Run(item.ID, func(t *testing.T) {
			names, err := experienceNativePermissionTools(item.PermissionProfile)
			if err != nil {
				t.Fatal(err)
			}
			id := experienceDigest("actual-frozen-permission:" + item.ID)
			record := r.createTask(t, ctx, id, item.Instruction, item.Resource, item.PermissionProfile)
			_, mandate, _, err := r.goals.ReadTaskAuthority(ctx, record.TaskID)
			if err != nil || len(mandate.Extensions) != 1 || len(mandate.Extensions[0].Tools) != len(names) || mandate.Budget.ModelTurns != policy.Mandate.ModelTurns || mandate.Budget.ExternalCalls != policy.Mandate.ExternalCalls || mandate.ExpiresAt.After(r.expires.Add(time.Microsecond)) || len(mandate.Effects) != len(names)-1 {
				t.Fatal("frozen profile changed the ordinary finite mandate")
			}
			grant, err := r.extensions.Control(experienceNativeActor(ctx, id+":read-grant"), extensions.ControlRequest{Operation: "task_access", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID}})
			if err != nil || grant.Access == nil || grant.Access.TaskRevision != record.Revision || grant.Access.ConstraintsRevision != record.ConstraintsRevision || grant.Access.ManifestHash != r.pack.Digest || grant.Access.Generation != r.generation || len(grant.Access.Tools) != len(names) || len(grant.Access.Resources) != 1 || grant.Access.Resources[0].Origin != egresstest.Origin || grant.Access.Resources[0].PathPrefix != "/project/"+item.Resource || grant.Access.ExpiresAt.After(r.expires.Add(time.Microsecond)) {
				t.Fatal("frozen profile changed current exact-resource extension authority")
			}
			for _, name := range names {
				found, inMandate := false, false
				for _, allowed := range grant.Access.Tools {
					found = found || allowed == name
				}
				for _, allowed := range mandate.Extensions[0].Tools {
					inMandate = inMandate || allowed == name
				}
				if !found || !inMandate {
					t.Fatal("ordinary grant omitted an exact frozen tool")
				}
			}
			if item.ID != "original-04" {
				return
			}
			r.acceptCurrentProfile(t, ctx, record)
			out := r.execute(t, ctx, record, id, item.Instruction, &experienceOriginalControlProvider{})
			if out.Result == nil || out.Result.RunID == "" || out.Result.CurrentState == commonv1.RunState_RUN_STATE_COMPLETED {
				t.Fatal("actual original04 permission did not preserve its unresolved failure")
			}
			evidence, err := r.journal.GetRunEvidence(ctx, out.Result.RunID)
			unknown := 0
			for _, action := range evidence {
				if action.EffectState == actionauth.StateUnknown {
					unknown++
				}
			}
			var finals int
			if err != nil || unknown != 1 || recipient.snapshot().Dispatches != 1 || len(recipient.snapshot().Entries) != 1 || pool.QueryRow(ctx, `SELECT count(*) FROM run_final_results WHERE run_id=$1`, out.Result.RunID).Scan(&finals) != nil || finals != 0 {
				t.Fatal("original04 authority replayed or invented confirmation/final")
			}
		})
	}
}

func (r *experienceNativeRuntime) execute(t *testing.T, ctx context.Context, record task.Record, id, text string, provider transport.ModelProvider) adaptiveNativeExecution {
	return r.executeInput(t, ctx, record, id, text, "", provider)
}

// Execute exactly the visible continuation admitted by the ordinary owner
// controller. Do not accept another input or reclaim the failed original.
func (r *experienceNativeRuntime) executeQueued(t *testing.T, ctx context.Context, record task.Record, inputID string, provider transport.ModelProvider) adaptiveNativeExecution {
	return r.executeInput(t, ctx, record, "", "", inputID, provider)
}

func (r *experienceNativeRuntime) executeInput(t *testing.T, ctx context.Context, record task.Record, id, text, inputID string, provider transport.ModelProvider) adaptiveNativeExecution {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, _, _, err := r.goals.ReadTaskAuthority(ctx, record.TaskID)
	if err != nil {
		t.Fatal("current typed assignment unavailable", err)
	}
	currentSources := experienceTimedRunSources{experienceRunSources: experienceRunSources{sources: experienceTimedCatalog{Store: r.cat, timing: r.timing}, catalog: r.cat, store: r.lessons}, timing: r.timing}
	fc := flow.Config{ProviderName: "antigravity", ModelName: "gemini-3.8-flash-high", OwnerID: "experience-native-worker", LeaseTTL: 120, Tasks: r.tasks, Budgets: r.budgets, MemorySources: currentSources, Completion: r.results, Checkpoints: r.inputs, PromptManager: r.pm, PromptAssembler: promptmgmt.NewAssembler(), ContextBudgetTokens: 65536, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, Tools: r.broker, ToolCatalog: r.broker, ActionAuthorizer: r.broker, OwnerGoals: r.goals, ActionMandateID: a.MandateID, ApprovalService: r.decisions}
	fc.ProfileStore = profileStoreAdapter{store: profile.NewStore(r.pool), sources: r.cat}
	fc = experienceNativeForegroundConfig(fc, providerfactory.Result{Provider: provider, ModelName: "gemini-3.8-flash-high"}, r.budgets, r.foreground)
	fc.MemoryScopeResolver = func(c context.Context) ([]memoryservice.Scope, error) {
		extra, err := r.cat.ExtraScopes(c, "memory_read")
		out := []memoryservice.Scope{}
		for _, s := range extra {
			out = append(out, memoryservice.Scope{Type: s.Type, ID: s.ID})
		}
		return out, err
	}
	if r.skillClient != nil {
		fc.Skills = r.skillClient
	}
	base := flow.NewService(r.sessions, session.NewPostgresLeaseManager(r.pool), r.runs, r.history, provider, fc, log)
	x := &adaptiveNativeExecutor{base: base, done: make(chan adaptiveNativeExecution, 1)}
	worker, err := durableinput.NewServiceWithExecutionTimeout(r.inputs, r.sessions, x, r.runs, log, 40*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if inputID == "" {
		payload, _ := json.Marshal(map[string]string{"text": text, "user_id": experienceNativeOwner, "task_id": record.TaskID, "intent": "conversation"})
		event := ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: record.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}
		acceptedBy := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: experienceNativeOwner, Channel: "web", SessionKey: record.SessionKey, DeliveryID: id + ":accept"})
		receipt, err := worker.Execute(acceptedBy, event)
		if err != nil {
			t.Fatal("ordinary owning input acceptance", err)
		}
		inputID = receipt.InputID
	} else {
		queued, err := r.inputs.Get(ctx, inputID)
		if err != nil || queued.Status != "queued" || queued.Attempts != 0 || queued.OwnerID != experienceNativeOwner || queued.TaskID != record.TaskID || queued.SessionKey != record.SessionKey || queued.RunID != "" {
			t.Fatal("exact fresh owner continuation required", err)
		}
	}
	if _, err = r.pool.Exec(ctx, `UPDATE accepted_inputs SET max_attempts=1 WHERE input_id=$1`, inputID); err != nil {
		t.Fatal("one actual original attempt required")
	}
	// Owner proof authorizes ingress and explicit controls. The worker must
	// exercise its own run token rather than inherit a caller's owner bypass.
	work, stop := context.WithCancel(owneridentity.WithActor(ctx, owneridentity.Actor{}))
	if _, ok := owneridentity.FromContext(work); ok {
		t.Fatal("foreground worker retained owner actor")
	}
	done := make(chan struct{})
	go func() { _ = worker.Run(work); close(done) }()
	var output adaptiveNativeExecution
	select {
	case output = <-x.done:
	case <-ctx.Done():
		stop()
		<-done
		t.Fatal("earlier collection deadline expired")
	}
	sealed := false
	for until := time.Now().Add(8 * time.Second); time.Now().Before(until); {
		var state string
		if r.pool.QueryRow(ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, inputID).Scan(&state) == nil && state != "running" && state != "queued" {
			sealed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	<-done
	if !sealed {
		t.Fatal("original input not sealed; later work must not reclaim it")
	}
	return output
}

// This owning verifier correlates actual journal effects, exact recipient
// identities and the current check observation. Hidden recipient persistence is
// never supplied to a model/lesson or used to turn UNKNOWN into confirmed.
func (r *experienceNativeRuntime) verifyPreparedOriginal(ctx context.Context, record task.Record, run, resource string, recipient *experienceRecipient) (task.Result, error) {
	effects, err := r.journal.GetRunEvidence(ctx, run)
	if err != nil {
		return task.Result{}, err
	}
	actual := recipient.snapshot()
	if actual.Config.Outage {
		return task.Result{}, fmt.Errorf("independent current verification unavailable")
	}
	var prepared *experienceRecipientEntry
	refs := []string{}
	for _, effect := range effects {
		if effect.EffectState != actionauth.StateConfirmed {
			return task.Result{}, fmt.Errorf("current action is not confirmed")
		}
		e, err := r.journal.GetEffect(ctx, effect.ActionID)
		if err != nil {
			return task.Result{}, err
		}
		refs = append(refs, e.EvidenceRefs...)
		if e.Binding.ToolName != "extension.project.prepare-patch" {
			continue
		}
		entry, ok := actual.Entries[e.IdempotencyKey]
		if !ok || entry.Fingerprint != e.Fingerprint || entry.State != "effect_confirmed" || entry.Resource != resource || !recipient.validProposal(resource, entry.Proposal) || prepared != nil {
			return task.Result{}, fmt.Errorf("one matching current artifact required")
		}
		prepared = &entry
	}
	if prepared == nil {
		return task.Result{}, fmt.Errorf("actual prepared artifact missing")
	}
	captured, err := r.history.CaptureRunTools(experienceNativeActor(ctx, "verify-capture:"+run), run, 2*experience.MaxActions)
	if err != nil {
		return task.Result{}, err
	}
	checked := false
	for _, call := range captured.Calls {
		if call.ToolName != "extension.project.check" || call.Status != "completed" {
			continue
		}
		var args struct {
			URL string `json:"url"`
		}
		var out struct {
			Body string `json:"body"`
		}
		var proof struct {
			ArtifactID      string `json:"artifact_id"`
			ProjectRevision string `json:"project_revision"`
			Passed          bool   `json:"checks_passed"`
			Applied         bool   `json:"applied"`
			Content         string `json:"prepared_content"`
		}
		if json.Unmarshal([]byte(call.ArgsJSON), &args) == nil && args.URL == egresstest.Origin+"/project/"+resource+"/patches/"+prepared.Artifact+"/checks" && json.Unmarshal([]byte(call.ResultJSON), &out) == nil && json.Unmarshal([]byte(out.Body), &proof) == nil && proof.ArtifactID == prepared.Artifact && proof.ProjectRevision == prepared.Proposal.ProjectRevision && proof.Passed && !proof.Applied && proof.Content == prepared.Proposal.Changes[0].After {
			checked = true
		}
	}
	if !checked || len(refs) == 0 {
		return task.Result{}, fmt.Errorf("matching actual unapplied artifact check missing")
	}
	return r.tasks.SaveVerifiedResult(experienceNativeActor(ctx, "verify:"+run), task.Result{TaskID: record.TaskID, TaskRevision: record.Revision, RunID: run, Summary: "Independent owner receipt and current artifact check; original remains unchanged", EvidenceRefs: refs, VerifiedCriteria: append([]string{}, record.CompletionCriteria...), Verifier: "owner", Verdict: "verified"})
}

type experienceOriginalControlProvider struct {
	run   string
	phase int
	ready string // explicit controlled protocol fault; never used by native transport
}

func (*experienceOriginalControlProvider) Name() string                    { return "antigravity" }
func (*experienceOriginalControlProvider) InferenceLimits() (int64, int64) { return 1048576, 32768 }
func (*experienceOriginalControlProvider) InferenceTimeout() time.Duration { return 300 * time.Second }
func (*experienceOriginalControlProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsToolCalls: true}, nil
}
func (*experienceOriginalControlProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (*experienceOriginalControlProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, fmt.Errorf("controlled fresh original has no approval/retry")
}
func (p *experienceOriginalControlProvider) emit(name string, args any) transport.EventStream {
	raw, _ := json.Marshal(args)
	e := transport.NewToolCallRequestedEvent(p.run, p.Name(), transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("controlled-%s-%d", experienceDigest(p.run)[:20], p.phase), ToolName: name, ArgsJSON: string(raw)})
	e.Usage = &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}
	ch := make(chan transport.TransportEvent, 1)
	ch <- e
	close(ch)
	return ch
}
func (p *experienceOriginalControlProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("actual source authority denied")
	}
	visible, _ := json.Marshal(q.InputItems)
	if !strings.Contains(string(visible), experienceNativePreference) {
		return nil, fmt.Errorf("actual model context omitted current accepted scoped preference")
	}
	p.run = q.Context.RunID
	p.phase = 1
	return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha"}), nil
}
func (p *experienceOriginalControlProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("actual source authority denied")
	}
	var envelope struct {
		Status string `json:"status"`
		Result struct {
			Body string `json:"body"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" {
		return nil, fmt.Errorf("actual current tool failed")
	}
	p.phase++
	switch p.phase {
	case 2:
		var inspected struct {
			Revision string                           `json:"project_revision"`
			Ready    string                           `json:"accepted_ready_value"`
			Files    []struct{ Path, Content string } `json:"files"`
		}
		if json.Unmarshal([]byte(envelope.Result.Body), &inspected) != nil || len(inspected.Files) != 1 {
			return nil, fmt.Errorf("actual inspect missing")
		}
		before := inspected.Files[0].Content
		if p.ready != "" {
			inspected.Ready = p.ready
		}
		body := map[string]any{"project_revision": inspected.Revision, "changes": []map[string]string{{"path": "README.md", "before": before, "after": strings.ReplaceAll(before, "ready=false\n", "ready="+inspected.Ready+"\n")}}}
		return p.emit("extension.project.prepare-patch", map[string]any{"url": egresstest.Origin + "/project/alpha/patches", "body": body}), nil
	case 3:
		var receipt struct {
			Artifact string `json:"artifact_id"`
		}
		if json.Unmarshal([]byte(envelope.Result.Body), &receipt) != nil || receipt.Artifact == "" {
			return nil, fmt.Errorf("actual artifact missing")
		}
		return p.emit("extension.project.check", map[string]string{"url": egresstest.Origin + "/project/alpha/patches/" + receipt.Artifact + "/checks"}), nil
	case 4:
		var checked struct {
			Passed  bool `json:"checks_passed"`
			Applied bool `json:"applied"`
		}
		if json.Unmarshal([]byte(envelope.Result.Body), &checked) != nil || !checked.Passed || checked.Applied {
			return nil, fmt.Errorf("actual check failed")
		}
		ch := make(chan transport.TransportEvent, 1)
		ch <- transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: "Подготовлен и проверен текущий артефакт; оригинал не изменён.", FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
		close(ch)
		return ch, nil
	}
	return nil, fmt.Errorf("controlled original exceeded fixed steps")
}

func TestExperienceNativeOrdinarySourcePipelineIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	anchor := filepath.Join(t.TempDir(), "anchor")
	m := catalog.NewFileManifest(anchor)
	if m.Initialize() != nil {
		t.Fatal("private anchor unavailable")
	}
	cat := catalog.New(pool, m)
	addresses := newExperienceNativeAddresses(t)
	var generation int64
	var environment string
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			dir := t.TempDir()
			recipient, err := newExperienceRecipient(dir, experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": fmt.Sprintf("Synthetic current source\nready=false\n<!-- current-%d -->\n", i), "beta": "Synthetic other resource\nready=false\n"}})
			if err != nil {
				t.Fatal(err)
			}
			defer recipient.close()
			r := newExperienceNativeRuntime(t, pool, cat, anchor, dir, recipient, addresses, experienceNativeSourcePolicy(), "Выполни текущее поручение; не применяй патчи; неизвестный эффект не является успехом.", experienceNativeArms[0], nil, time.Now().Add(time.Hour))
			if i == 0 {
				generation, environment = r.generation, r.environment
			} else if generation != r.generation || environment != r.environment {
				t.Fatal("fresh identical runtime changed frozen environment/contract generation")
			}
			id := experienceDigest(fmt.Sprint("controlled-current-task", i))
			record := r.createTask(t, ctx, id, "Prepare and independently check an unapplied current README artifact", "alpha", "prepare_and_verify_exact_resource")
			r.acceptCurrentProfile(t, ctx, record)
			out := r.execute(t, ctx, record, id, "Inspect alpha, prepare readiness true, and check only the current unapplied artifact", &experienceOriginalControlProvider{})
			if out.Err != nil || out.Result == nil || out.Result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED {
				t.Fatal("actual ordinary source pipeline", out.Err, out.Result)
			}
			if _, err = r.verifyPreparedOriginal(ctx, record, out.Result.RunID, "alpha", recipient); err != nil {
				t.Fatal("independent original verifier", err)
			}
		})
	}
}
