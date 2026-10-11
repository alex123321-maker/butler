package workspaceexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sourceActionKey struct{}

// Explicit approved fixture intent, not production Broker authorization.
type sourceAction struct {
	context               SourceContext
	operation, path, hash string
	bytes                 int64
}
type sourceAdmission struct{ provenance *Provenance }

func (g sourceAdmission) CheckWorkspaceTx(ctx context.Context, tx pgx.Tx, a workspace.Access) (resources.Binding, error) {
	c, ok := ctx.Value(sourceActionKey{}).(sourceAction)
	if !ok || a.Ref != c.context.ref || a.Operation != c.operation || a.Path != c.path || a.Operation == "write" && (a.ContentSHA256 != c.hash || a.Bytes != c.bytes) {
		return resources.Binding{}, ErrDenied
	}
	if err := g.provenance.CheckTx(ctx, tx, c.context, a.Proof.LeaseID); err != nil {
		return resources.Binding{}, err
	}
	return worker(ctx, tx, a.Ref)
}

type sourceEvidence struct {
	provenance *Provenance
	observer   *workspace.Observer
	fail       bool
}

func (e *sourceEvidence) RecordNamespaceTx(ctx context.Context, tx pgx.Tx, a workspace.Access) (string, error) {
	o, err := e.observer.NamespaceTx(ctx, tx, a)
	if err != nil {
		return "", err
	}
	return e.record(ctx, tx, a, o)
}
func (e *sourceEvidence) RecordWriteTx(ctx context.Context, tx pgx.Tx, a workspace.Access, s workspace.Snapshot) (string, error) {
	o, err := e.observer.FileTx(ctx, tx, a)
	if err != nil || o.Snapshot != s {
		return "", workspace.ErrDrift
	}
	return e.record(ctx, tx, a, o)
}
func (e *sourceEvidence) record(ctx context.Context, tx pgx.Tx, a workspace.Access, o workspace.Observation) (string, error) {
	c, ok := ctx.Value(sourceActionKey{}).(sourceAction)
	if !ok {
		return "", ErrDenied
	}
	id, err := e.provenance.ConfirmTx(ctx, tx, c.context, a, o)
	if err == nil && e.fail {
		return "", ErrDenied
	}
	return id, err
}

type noOwnerRecovery struct{}

func (noOwnerRecovery) CheckReconciliationTx(context.Context, pgx.Tx, resources.Decision) error {
	return resources.ErrDenied
}

type sourceFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	catalog    *catalog.Store
	provenance *Provenance
	resource   *resources.Store
	evidence   *sourceEvidence
	store      *workspace.Store
	purge      *workspace.DeletionRuntime
	source     catalog.Source
	root       string
	binding    resources.Binding
}

func newSourceFixture(t *testing.T) *sourceFixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	t.Cleanup(cancel)
	_, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('wp-session','wp-owner','internal');INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('wp-task','wp-session','wp-owner','bounded artifacts');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('wp-run','wp-session','fixture','tool_running','tool_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'wp-task',1);INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('wp-session','wp-run','wp-lease',1,clock_timestamp()+interval '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "fixture"})
	src, err := c.Put(owner, catalog.PutSource{ExternalKey: "workspace-original", Kind: "document", ScopeType: "user", ScopeID: "wp-owner", Title: "Original", Body: "synthetic original workspace input"})
	if err != nil {
		t.Fatal(err)
	}
	ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "wp-session", RunID: "wp-run", LeaseID: "wp-lease", FenceEpoch: 1})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = c.AccessTx(ctx, tx, []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, "memory_read"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := NewProvenance(pool, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	rs, err := resources.New(resources.WorkerGuard{}, noOwnerRecovery{})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	o, err := workspace.NewObserver(root, workspace.DefaultLimits().MaxFileBytes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	e := &sourceEvidence{provenance: p, observer: o}
	g, err := workspace.NewResourceGuard(rs, sourceAdmission{p}, e)
	if err != nil {
		t.Fatal(err)
	}
	s, err := workspace.New(pool, root, g, workspace.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	purge, err := workspace.NewDeletionRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = purge.Close() })
	b := resources.Binding{OwnerID: "wp-owner", RunID: "wp-run", SessionKey: "wp-session", TaskID: "wp-task", TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: 1, Deadline: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond)}
	return &sourceFixture{ctx, pool, c, p, rs, e, s, purge, src, root, b}
}
func (f *sourceFixture) prepare(t *testing.T, ref workspace.Ref, operation, path string, data []byte) (context.Context, workspace.LeaseProof) {
	t.Helper()
	ctx := f.ctx
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	c, err := f.provenance.PrepareTx(ctx, tx, ref, operation == "create")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.resource.RegisterTx(ctx, tx, f.binding, workspace.ResourceID(ref.ID)); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT version FROM execution_resources WHERE resource_id=$1`, workspace.ResourceID(ref.ID)).Scan(&version); err != nil {
		t.Fatal(err)
	}
	mode := resources.Write
	if operation == "read" {
		mode = resources.Read
	}
	proofs, err := f.resource.AcquireTx(ctx, tx, f.binding, []resources.Request{{ResourceID: workspace.ResourceID(ref.ID), Mode: mode, ExpectedVersion: version}}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p := proofs[0]
	if mode == resources.Write {
		if err = f.resource.ClaimTx(ctx, tx, f.binding, p); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	ctx = context.WithValue(ctx, sourceActionKey{}, sourceAction{c, operation, path, hex.EncodeToString(h[:]), int64(len(data))})
	return ctx, workspace.LeaseProof{LeaseID: p.LeaseID, ResourceID: p.ResourceID, FenceEpoch: p.FenceEpoch, ExpectedResourceVersion: p.ExpectedResourceVersion, RunID: p.RunID, SessionKey: p.SessionKey, TaskID: p.TaskID}
}
func (f *sourceFixture) create(t *testing.T) workspace.Ref {
	t.Helper()
	b := f.binding
	r, err := workspace.Allocate(workspace.Binding{OwnerID: b.OwnerID, SessionKey: b.SessionKey, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision})
	if err != nil {
		t.Fatal(err)
	}
	ctx, p := f.prepare(t, r, "create", "", nil)
	if err = f.store.Create(ctx, r, p); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWorkspaceProvenancePersistsOriginalGraphAcrossRunIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	body := []byte("package main // synthetic artifact")
	ctx, proof := f.prepare(t, ref, "write", "main.go", body)
	s, err := f.store.Write(ctx, ref, proof, workspace.Snapshot{Path: "main.go"}, body)
	if err != nil {
		t.Fatal(err)
	}
	var sourceID, kind, scope, task, sourceBody string
	var sequence int64
	if err = f.pool.QueryRow(ctx, `SELECT p.sequence,s.source_id,s.source_kind,s.scope_type,s.scope_id,r.body FROM code_workspace_sources p JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE p.workspace_id=$1 ORDER BY p.sequence DESC LIMIT 1`, ref.ID).Scan(&sequence, &sourceID, &kind, &scope, &task, &sourceBody); err != nil || sequence != 2 || kind != "model_candidate" || scope != "task" || task != "wp-task" || sourceBody != "" {
		t.Fatal("provenance widened scope/copied body", sequence, kind, scope, sourceBody, err)
	}
	var original bool
	if err = f.pool.QueryRow(ctx, `WITH RECURSIVE graph AS(SELECT parent_source_id FROM memory_source_dependencies WHERE child_source_id=$1 UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN graph g ON g.parent_source_id=d.child_source_id) SELECT EXISTS(SELECT 1 FROM graph WHERE parent_source_id=$2)`, sourceID, f.source.ID).Scan(&original); err != nil || !original {
		t.Fatal("original graph missing", err)
	}
	old := ctx.Value(sourceActionKey{}).(sourceAction).context
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.provenance.CheckTx(ctx, tx, old, proof.LeaseID); err == nil {
		t.Fatal("committed context reused as own invisible publication")
	}
	_ = tx.Rollback(context.Background())
	// A new actual run has no old model/source context. The durable copy still
	// independently checks/pins its original provenance instead of losing it.
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET status='completed',current_state='completed' WHERE run_id='wp-run';INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('wp-run2','wp-session','fixture2','tool_running','tool_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'wp-task',1);UPDATE session_ownership SET run_id='wp-run2',lease_id='wp-lease2',fence_epoch=2 WHERE session_key='wp-session'`); err != nil {
		t.Fatal(err)
	}
	f.ctx = ownership.WithToken(f.ctx, nil, ownership.Token{SessionKey: "wp-session", RunID: "wp-run2", LeaseID: "wp-lease2", FenceEpoch: 2})
	f.binding.RunID = "wp-run2"
	f.binding.LeaseEpoch = 2
	ctx, proof = f.prepare(t, ref, "read", "main.go", nil)
	file, err := f.store.Read(ctx, ref, proof, "main.go")
	if err != nil || file.Snapshot != s || string(file.Content) != string(body) {
		t.Fatal("new run cannot read current copy", err)
	}
	var pinned bool
	if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_run_sources WHERE run_id='wp-run2' AND source_id=$1)`, f.source.ID).Scan(&pinned); err != nil || !pinned {
		t.Fatal("new run lost original source", err)
	}
	encoded, _ := json.Marshal(old)
	if strings.Contains(string(encoded), f.source.ID) || string(encoded) != "{}" {
		t.Fatal("private context escaped")
	}
}

func TestWorkspaceDeletedSourceBlocksAndPurgesActualCopiesIntegration(t *testing.T) {
	for _, action := range []string{"delete", "revoke"} {
		t.Run(action, func(t *testing.T) {
			f := newSourceFixture(t)
			ref := f.create(t)
			body := []byte("package main // synthetic original")
			ctx, proof := f.prepare(t, ref, "write", "main.go", body)
			if _, err := f.store.Write(ctx, ref, proof, workspace.Snapshot{Path: "main.go"}, body); err != nil {
				t.Fatal(err)
			}
			ctx, proof = f.prepare(t, ref, "read", "main.go", nil)
			if file, err := f.store.Read(ctx, ref, proof, "main.go"); err != nil || string(file.Content) != string(body) {
				t.Fatal("pre-deletion read was not authorized", err)
			}
			owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "delete"})
			if _, err := f.catalog.Delete(owner, f.source.ID, action, f.source.Revision); err != nil {
				t.Fatal(err)
			}
			var state, queued string
			if err := f.pool.QueryRow(f.ctx, `SELECT w.state,d.status FROM code_workspaces w JOIN code_workspace_deletions d USING(workspace_id) WHERE workspace_id=$1`, ref.ID).Scan(&state, &queued); err != nil || state != "blocked" || queued != "pending" {
				t.Fatal("deletion did not atomically block/queue", state, queued, err)
			}
			if _, err := f.store.Read(ctx, ref, proof, "main.go"); err == nil {
				t.Fatal("deleted artifact readable")
			}
			worked, err := f.provenance.PurgeDeleted(f.ctx, f.purge)
			if err != nil || !worked {
				t.Fatal("actual erasure", worked, err)
			}
			if _, err = os.Lstat(filepath.Join(f.root, ref.ID)); !os.IsNotExist(err) {
				t.Fatal("deleted body still on disk", err)
			}
			worked, err = f.provenance.PurgeDeleted(f.ctx, f.purge)
			if err != nil || worked {
				t.Fatal("erasure not idempotent", worked, err)
			}
			if err = f.pool.QueryRow(f.ctx, `SELECT status FROM code_workspace_deletions WHERE workspace_id=$1`, ref.ID).Scan(&queued); err != nil || queued != "done" {
				t.Fatal("erasure not durable", queued, err)
			}
		})
	}
}

func TestWorkspaceProvenanceFailureAfterRenamePublishesNoSourcesIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	f.evidence.fail = true
	body := []byte("package main // uncertain fixture")
	ctx, proof := f.prepare(t, ref, "write", "main.go", body)
	if _, err := f.store.Write(ctx, ref, proof, workspace.Snapshot{Path: "main.go"}, body); !errors.Is(err, workspace.ErrOutcomeUnknown) {
		t.Fatal("unknown physical effect reported as success", err)
	}
	var copies, derived, files int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM code_workspace_sources WHERE workspace_id=$1),(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'),(SELECT count(*) FROM code_workspace_files WHERE workspace_id=$1)`, ref.ID).Scan(&copies, &derived, &files); err != nil || copies != 1 || derived != 1 || files != 0 {
		t.Fatal("failed publication leaked provenance", copies, derived, files, err)
	}
	actual, err := os.ReadFile(filepath.Join(f.root, ref.ID, "tree", "main.go"))
	if err != nil || string(actual) != string(body) {
		t.Fatal("invented filesystem rollback", err)
	}
}
