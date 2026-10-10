package extensions

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func extensionDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires isolated *_test database")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("extensions_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if _, err = pool.Exec(ctx, `CREATE TABLE tasks(task_id text PRIMARY KEY,owner_id text NOT NULL,status text NOT NULL,revision bigint NOT NULL,constraints_revision bigint NOT NULL,execution_policy text NOT NULL DEFAULT 'normal'); INSERT INTO tasks(task_id,owner_id,status,revision,constraints_revision) VALUES('task','owner','active',1,1);
 CREATE TABLE runs(run_id text PRIMARY KEY,task_id text,task_revision bigint);
 CREATE TABLE session_ownership(session_key text PRIMARY KEY,run_id text NOT NULL,lease_id text NOT NULL,fence_epoch bigint NOT NULL,expires_at timestamptz NOT NULL);
 INSERT INTO runs VALUES('run','task',1);
 INSERT INTO session_ownership VALUES('web:owner','run','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql", "037_execution_budgets.up.sql", "038_owner_goals_mandates.up.sql", "038_owner_purpose_policies.up.sql", "050_reviewed_extensions.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	return pool
}
func ownerContext() context.Context {
	return owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture-owner-proof"})
}
func extensionFixture(t *testing.T, pool *pgxpool.Pool, policy budget.Policy) (*Store, Package) {
	t.Helper()
	p, err := parseFixture(t, fixtureManifest())
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, "owner", map[string]Profile{"reference-http": {Target: "extension-runtime:50051", Dependency: p.Manifest.Dependencies[0]}}, budget.NewController(pool, policy))
	return store, p
}
func configuredFixture(t *testing.T, s *Store, p Package) Record {
	t.Helper()
	ctx := ownerContext()
	r, err := s.Stage(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Review(ctx, r.ID, p.Digest); err != nil {
		t.Fatal(err)
	}
	r, err = s.Configure(ctx, r.ID, p.Digest, r.Generation, map[string]string{"origin": "https://extension.example", "collection": "fixture"}, nil, []string{"extension.notes.read", "extension.notes.write"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func readyFixture(t *testing.T, s *Store, r Record) Record {
	t.Helper()
	ctx := ownerContext()
	probe, _, err := s.BeginProbe(ctx, r.ID, r.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishProbe(ctx, probe, true, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	r, err = s.SetEnabled(ctx, r.ID, r.Generation, true)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestReviewedLifecycleKeepsStatesHistoryAndVersionPinsIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	ctx := ownerContext()
	if _, err := s.Stage(context.Background(), p); !errors.Is(err, ErrAccess) {
		t.Fatalf("untrusted installation accepted: %v", err)
	}
	r, err := s.Stage(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "staged" || r.Reviewed || r.Available || r.Health != "unknown" {
		t.Fatalf("staging implied readiness: %+v", r)
	}
	if _, err = s.SetEnabled(ctx, r.ID, r.Generation, true); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unreviewed extension enabled")
	}
	if err = s.Review(ctx, r.ID, p.Digest); err != nil {
		t.Fatal(err)
	}
	r, err = s.Configure(ctx, r.ID, p.Digest, r.Generation, map[string]string{"origin": "https://extension.example"}, nil, []string{"extension.notes.read"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.HealthReason, "collection") {
		t.Fatalf("missing setting reason absent: %s", r.HealthReason)
	}
	if _, _, err = s.BeginProbe(ctx, r.ID, r.Generation); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing configuration reached egress admission")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid probe consumed or persisted dispatch")
	}
	r, err = s.Configure(ctx, r.ID, p.Digest, r.Generation, map[string]string{"origin": "https://extension.example", "collection": "fixture"}, nil, []string{"extension.notes.read"})
	if err != nil {
		t.Fatal(err)
	}
	r = readyFixture(t, s, r)
	if !r.Available || !r.Reviewed || r.Status != "enabled" || r.Health != "healthy" {
		t.Fatalf("validated states disagree: %+v", r)
	}
	a := TaskAccess{TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1, ManifestHash: p.Digest, Generation: r.Generation, Tools: []string{"extension.notes.read"}, Resources: []actionauth.ResourceScope{{Origin: "https://extension.example", PathPrefix: "/notes"}}, ExpiresAt: time.Now().Add(time.Hour)}
	if err = s.GrantTask(ctx, r.ID, a); err != nil {
		t.Fatal(err)
	}
	oldAccess, err := s.TaskAccess(ctx, "task", r.ID)
	if err != nil || oldAccess.Revision != 1 {
		t.Fatal(err)
	}
	// Update does not modify the immutable version or activate a staged package.
	m := fixtureManifest()
	m.Version = "2.0.0"
	m.Operations[0].Description = "Read a note using reviewed version 2"
	updated, err := parseFixture(t, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Stage(ctx, updated); err != nil {
		t.Fatal(err)
	}
	same, err := s.Get(ctx, r.ID)
	if err != nil || same.ManifestHash != p.Digest || same.Generation != r.Generation {
		t.Fatal("staging silently changed contract")
	}
	mutated := fixtureManifest()
	mutated.Description = "changed bytes under existing version"
	conflict, _ := parseFixture(t, mutated)
	if _, err = s.Stage(ctx, conflict); !errors.Is(err, ErrConflict) {
		t.Fatal("same version changed immutable digest")
	}
	if err = s.Review(ctx, r.ID, updated.Digest); err != nil {
		t.Fatal(err)
	}
	r, err = s.Configure(ctx, r.ID, updated.Digest, r.Generation, r.Configuration, nil, r.AllowedTools)
	if err != nil {
		t.Fatal(err)
	}
	if r.Available || r.Status != "disabled" || r.ManifestHash == oldAccess.ManifestHash || r.Generation == oldAccess.Generation {
		t.Fatal("upgrade retained old authority/readiness")
	}
	// Rollback is another disabled configuration selection and requires a probe.
	r, err = s.Configure(ctx, r.ID, p.Digest, r.Generation, r.Configuration, nil, r.AllowedTools)
	if err != nil {
		t.Fatal(err)
	}
	if r.Available || r.Generation == oldAccess.Generation {
		t.Fatal("rollback revived old permissions")
	}
	versions, err := s.Versions(ctx, r.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("history lost: %d %v", len(versions), err)
	}
	if _, err = pool.Exec(ctx, `UPDATE extension_versions SET version='9.0.0' WHERE manifest_hash=$1`, p.Digest); err == nil {
		t.Fatal("durable manifest mutated")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "050_reviewed_extensions.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("rollback deleted durable evidence")
	}
}
func TestProbeCASQuotaAndRestartNeverReplayIntegration(t *testing.T) {
	pool := extensionDB(t)
	policy := budget.DefaultPolicy()
	policy.Day.ExternalCalls = 1
	policy.Conversation.ExternalCalls = 1
	s, p := extensionFixture(t, pool, policy)
	ctx := ownerContext()
	r := configuredFixture(t, s, p)
	probe, _, err := s.BeginProbe(ctx, r.ID, r.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if probe.URL != "https://extension.example/notes/health" {
		t.Fatal("probe was model-controlled")
	}
	r, err = s.Configure(ctx, r.ID, p.Digest, r.Generation, r.Configuration, nil, r.AllowedTools)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishProbe(ctx, probe, true, strings.Repeat("a", 64)); !errors.Is(err, ErrConflict) {
		t.Fatal("stale probe published health")
	}
	if _, _, err = s.BeginProbe(ctx, r.ID, r.Generation); err == nil {
		t.Fatal("reconfiguration reset owner-day allowance")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM extension_probes`).Scan(&count); err != nil || count != 1 {
		t.Fatal("quota denial persisted external attempt")
	}
	if _, err = pool.Exec(ctx, `UPDATE extension_probes SET started_at=clock_timestamp()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	restarted, _ := extensionFixture(t, pool, policy)
	if err = restarted.RecoverProbes(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT state FROM extension_probes`).Scan(&state); err != nil || state != "unknown" {
		t.Fatal("restart made unknown probe replayable")
	}
	r, err = restarted.Get(ctx, r.ID)
	if err != nil || r.Available || r.Health == "healthy" {
		t.Fatal("interrupted validation enabled extension")
	}
}
func TestConfigurationRejectsSecretAndInternalTargetsBeforeProbeIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	ctx := ownerContext()
	r := configuredFixture(t, s, p)
	for _, values := range []map[string]string{{"origin": "http://127.0.0.1", "collection": "fixture"}, {"origin": "https://extension.example/notes", "collection": "fixture"}, {"origin": "https://extension.example", "collection": "fixture", "password": "private"}, {"origin": "https://extension.example", "collection": "fixture\nAuthorization: secret"}} {
		if _, err := s.Configure(ctx, r.ID, p.Digest, r.Generation, values, nil, r.AllowedTools); err == nil {
			t.Fatal("unbounded configuration accepted")
		}
	}
	if _, err := s.Configure(ctx, r.ID, p.Digest, r.Generation, r.Configuration, map[string]actionauth.CredentialRef{"unknown": {Alias: "private", Field: "token"}}, r.AllowedTools); err == nil {
		t.Fatal("undeclared secret slot accepted")
	}
}

func TestConcurrentProbeAdmissionAndInterruptedHealthIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	r := configuredFixture(t, s, p)
	start := make(chan struct{})
	results := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, _, err := s.BeginProbe(ownerContext(), r.ID, r.Generation)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	admitted := 0
	for err := range results {
		if err == nil {
			admitted++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	var probes, reservations int
	if err := pool.QueryRow(ownerContext(), `SELECT count(*) FROM extension_probes`).Scan(&probes); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ownerContext(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || admitted != 1 || probes != 1 || reservations != 1 {
		t.Fatalf("parallel probes created multiple health attempts/charges: admitted=%d probes=%d reservations=%d error=%v", admitted, probes, reservations, err)
	}
	if _, err := pool.Exec(ownerContext(), `UPDATE extension_probes SET started_at=clock_timestamp()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverProbes(ownerContext()); err != nil {
		t.Fatal(err)
	}
	r, err := s.Get(ownerContext(), r.ID)
	if err != nil || r.Available || r.HealthReason != "probe_unknown" {
		t.Fatal("recovery published ambiguous readiness")
	}
	probe, _, err := s.BeginProbe(ownerContext(), r.ID, r.Generation)
	if err != nil {
		t.Fatal("fresh owner validation could not replace interrupted attempt")
	}
	if err := s.FinishProbe(ownerContext(), probe, true, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestControlManifestTextPreservesExactReviewedBytesIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	raw := "\n " + strings.ReplaceAll(string(p.Raw), ",", ",\n ") + "\n"
	exact, err := Parse(strings.NewReader(raw))
	if err != nil || exact.Digest == p.Digest {
		t.Fatal("fixture did not distinguish exact reviewed bytes")
	}
	token := strings.Repeat("private-fixture-service-", 2)
	backend := httptest.NewServer(NewControlHandler(token, s, nil))
	defer backend.Close()
	client := NewControlClient(backend.URL, token)
	response, err := client.Control(ownerContext(), ControlRequest{Operation: "stage", ManifestText: raw})
	if err != nil || response.StagedHash != exact.Digest {
		t.Fatal("private relay normalized manifest bytes")
	}
	historical, err := s.ManifestByHash(context.Background(), response.StagedHash)
	if err != nil || string(historical.Raw) != raw {
		t.Fatal("stored provenance lost exact bytes")
	}
	if _, err = s.Control(ownerContext(), ControlRequest{Operation: "stage", ManifestText: raw, ManifestJSON: p.Raw}, nil); !errors.Is(err, ErrManifest) {
		t.Fatal("ambiguous manifest source accepted")
	}
	duplicate := strings.Replace(raw, `"format":`, `"format":"forged","format":`, 1)
	if _, err = s.Control(ownerContext(), ControlRequest{Operation: "stage", ManifestText: duplicate}, nil); !errors.Is(err, ErrManifest) {
		t.Fatal("text transport erased duplicate manifest member")
	}
}
