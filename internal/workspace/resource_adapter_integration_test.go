package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type resourceActionKey struct{}

// Exact, explicitly approved fixture actions and actual Catalog snapshots. This
// proves the adapter boundary, not production Broker canonical authorization.
type adapterAdmission struct {
	catalog      *catalog.Store
	claimOnCheck bool
	resources    *resources.Store
}

func (a *adapterAdmission) CheckWorkspaceTx(ctx context.Context, tx pgx.Tx, x Access) (resources.Binding, error) {
	var b resources.Binding
	id, _ := ctx.Value(resourceActionKey{}).(string)
	var operation, path, hash, sourceID, sourceHash string
	var bytes, revision, accessRevision int64
	err := tx.QueryRow(ctx, `SELECT owner_id,run_id,session_key,task_id,task_revision,constraints_revision,owner_epoch,deadline,operation,path,body_hash,bytes,source_id,source_revision,source_access_revision,source_hash FROM fixture_workspace_actions WHERE action_id=$1`, id).Scan(&b.OwnerID, &b.RunID, &b.SessionKey, &b.TaskID, &b.TaskRevision, &b.ConstraintsRevision, &b.LeaseEpoch, &b.Deadline, &operation, &path, &hash, &bytes, &sourceID, &revision, &accessRevision, &sourceHash)
	if err != nil {
		return b, ErrDenied
	}
	if err = (resources.WorkerGuard{}).CheckTx(ctx, tx, b); err != nil {
		return b, err
	}
	var current bool
	err = tx.QueryRow(ctx, `SELECT resource_id=$2 AND state='approved' AND deadline>clock_timestamp() FROM fixture_workspace_actions WHERE action_id=$1 FOR SHARE`, id, x.Proof.ResourceID).Scan(&current)
	if err != nil || !current || operation != x.Operation || path != x.Path {
		return b, ErrDenied
	}
	if operation == "write" && (hash != x.ContentSHA256 || bytes != x.Bytes) {
		return b, ErrDenied
	}
	if operation == "read" && x.ContentSHA256 != "" && (hash != x.ContentSHA256 || bytes != x.Bytes) {
		return b, ErrDenied
	}
	if err = a.catalog.CheckRestoreTx(ctx, tx); err != nil {
		return b, err
	}
	err = tx.QueryRow(ctx, `SELECT s.owner_id=$2 AND s.status='active' AND s.head_revision=$3 AND s.access_revision=$4 AND r.content_hash=$5 FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1 FOR SHARE OF s,r`, sourceID, b.OwnerID, revision, accessRevision, sourceHash).Scan(&current)
	if err != nil || !current {
		return b, ErrDenied
	}
	if a.claimOnCheck {
		if err = a.resources.ClaimTx(ctx, tx, b, resourceProof(x.Proof)); err != nil {
			return b, err
		}
	}
	return b, nil
}

// Fixed operator root, real filesystem observation and exact source/action
// records. There is no model-supplied path, evidence or successful verdict.
type adapterEvidence struct {
	observer              *Observer
	failCreate, failWrite bool
	before                func(context.Context, pgx.Tx, Access) error
}

func (e *adapterEvidence) record(ctx context.Context, tx pgx.Tx, a Access, o Observation) (string, error) {
	id := "evidence:" + a.Proof.LeaseID
	action, _ := ctx.Value(resourceActionKey{}).(string)
	s := o.Snapshot
	_, err := tx.Exec(ctx, `INSERT INTO fixture_workspace_evidence(evidence_id,action_id,resource_id,lease_id,kind,path,file_version,hash,bytes,observation_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, action, a.Proof.ResourceID, a.Proof.LeaseID, o.Kind, s.Path, s.Version, s.SHA256, s.Bytes, o.Digest)
	return id, err
}
func (e *adapterEvidence) RecordNamespaceTx(ctx context.Context, tx pgx.Tx, a Access) (string, error) {
	if e.before != nil {
		if err := e.before(ctx, tx, a); err != nil {
			return "", err
		}
	}
	o, err := e.observer.NamespaceTx(ctx, tx, a)
	if err != nil {
		return "", err
	}
	id, err := e.record(ctx, tx, a, o)
	if err != nil {
		return "", err
	}
	if e.failCreate {
		return "", ErrDenied
	}
	return id, nil
}
func (e *adapterEvidence) RecordWriteTx(ctx context.Context, tx pgx.Tx, a Access, s Snapshot) (string, error) {
	if e.before != nil {
		if err := e.before(ctx, tx, a); err != nil {
			return "", err
		}
	}
	o, err := e.observer.FileTx(ctx, tx, a)
	if err != nil || o.Snapshot != s {
		return "", ErrDrift
	}
	id, err := e.record(ctx, tx, a, o)
	if err != nil {
		return "", err
	}
	if e.failWrite {
		return "", ErrDenied
	}
	return id, nil
}

type adapterOwnerGuard struct{}

func (adapterOwnerGuard) CheckReconciliationTx(ctx context.Context, tx pgx.Tx, d resources.Decision) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != d.OwnerID {
		return resources.ErrDenied
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND resource_id=$3 AND lease_id=$4 AND version=$5 AND executor_stopped AND actual_effect_observed FROM fixture_independent_reconciliation WHERE evidence_id=$1 FOR SHARE`, d.EvidenceID, d.OwnerID, d.ResourceID, d.LeaseID, d.ExpectedVersion).Scan(&valid)
	if err != nil || !valid {
		return resources.ErrDenied
	}
	// The fixture executor is synchronous and has already returned. Its durable
	// run/fence must also be invalidated; a boolean evidence claim alone is not
	// proof that a still-authorized executor has stopped.
	err = tx.QueryRow(ctx, `SELECT r.current_state='cancelled' AND r.status='cancelled' AND o.fence_epoch>l.owner_epoch FROM execution_resource_leases l JOIN runs r ON r.run_id=l.run_id JOIN session_ownership o ON o.session_key=l.session_key WHERE l.lease_id=$1 AND l.resource_id=$2`, d.LeaseID, d.ResourceID).Scan(&valid)
	if err != nil || !valid {
		return resources.ErrDenied
	}
	return nil
}

type adapterFixture struct {
	ctx       context.Context
	pool      *pgxpool.Pool
	catalog   *catalog.Store
	source    catalog.Source
	resource  *resources.Store
	guard     *ResourceGuard
	admission *adapterAdmission
	evidence  *adapterEvidence
	store     *Store
	root      string
	counter   int
}

func newAdapterFixture(t *testing.T) *adapterFixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	t.Cleanup(cancel)
	_, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('ws-s1','ws-owner','internal'),('ws-s2','ws-owner','internal');
 INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('ws-t1','ws-s1','ws-owner','bounded copy'),('ws-t2','ws-s2','ws-owner','independent copy');
 INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('ws-r1','ws-s1','fixture1','tool_running','tool_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'ws-t1',1),('ws-r2','ws-s2','fixture2','tool_running','tool_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'ws-t2',1);
 INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('ws-s1','ws-r1','worker-1',1,clock_timestamp()+interval '1 hour'),('ws-s2','ws-r2','worker-2',1,clock_timestamp()+interval '1 hour');
 CREATE TABLE fixture_workspace_actions(action_id text PRIMARY KEY,owner_id text,run_id text,session_key text,task_id text,task_revision bigint,constraints_revision bigint,owner_epoch bigint,deadline timestamptz,resource_id text,operation text,path text,body_hash text,bytes bigint,source_id text,source_revision bigint,source_access_revision bigint,source_hash text,state text);
 CREATE TABLE fixture_workspace_evidence(evidence_id text PRIMARY KEY,action_id text,resource_id text,lease_id text,kind text,path text,file_version bigint,hash text,bytes bigint,observation_hash text);
 CREATE TABLE fixture_independent_reconciliation(evidence_id text PRIMARY KEY,owner_id text,resource_id text,lease_id text,version bigint,executor_stopped boolean,actual_effect_observed boolean);`)
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "ws-owner", Channel: "web", DeliveryID: "fixture"})
	source, err := c.Put(owner, catalog.PutSource{ExternalKey: "workspace-source", Kind: "document", ScopeType: "user", ScopeID: "ws-owner", Title: "Bounded source", Body: "source-aware approved copy"})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := resources.New(resources.WorkerGuard{}, adapterOwnerGuard{})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	admission := &adapterAdmission{catalog: c, resources: rs}
	observer, err := NewObserver(root, DefaultLimits().MaxFileBytes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observer.Close() })
	evidence := &adapterEvidence{observer: observer}
	guard, err := NewResourceGuard(rs, admission, evidence)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(pool, root, guard, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &adapterFixture{ctx, pool, c, source, rs, guard, admission, evidence, store, root, 0}
}
func (f *adapterFixture) worker(i int) (context.Context, resources.Binding) {
	b := resources.Binding{OwnerID: "ws-owner", RunID: fmt.Sprintf("ws-r%d", i), SessionKey: fmt.Sprintf("ws-s%d", i), TaskID: fmt.Sprintf("ws-t%d", i), TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: 1, Deadline: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond)}
	ctx := ownership.WithToken(f.ctx, nil, ownership.Token{SessionKey: b.SessionKey, RunID: b.RunID, LeaseID: fmt.Sprintf("worker-%d", i), FenceEpoch: 1})
	return ctx, b
}
func (f *adapterFixture) action(t *testing.T, ctx context.Context, b resources.Binding, ref Ref, operation, path string, data []byte) context.Context {
	t.Helper()
	f.counter++
	id := fmt.Sprintf("action-%d", f.counter)
	hash := ""
	if operation != "create" {
		hash = digest(data)
	}
	_, err := f.pool.Exec(ctx, `INSERT INTO fixture_workspace_actions VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'approved')`, id, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, b.LeaseEpoch, b.Deadline, ResourceID(ref.ID), operation, path, hash, len(data), f.source.ID, f.source.Revision, f.source.AccessRevision, f.source.Hash)
	if err != nil {
		t.Fatal(err)
	}
	return context.WithValue(ctx, resourceActionKey{}, id)
}
func (f *adapterFixture) tx(t *testing.T, ctx context.Context, fn func(pgx.Tx) error) error {
	t.Helper()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func workspaceProof(p resources.Proof) LeaseProof {
	return LeaseProof{LeaseID: p.LeaseID, ResourceID: p.ResourceID, FenceEpoch: p.FenceEpoch, ExpectedResourceVersion: p.ExpectedResourceVersion, RunID: p.RunID, SessionKey: p.SessionKey, TaskID: p.TaskID}
}
func (f *adapterFixture) acquire(t *testing.T, ctx context.Context, b resources.Binding, ref Ref, version int64, mode resources.Mode, claim bool) LeaseProof {
	t.Helper()
	var p resources.Proof
	err := f.tx(t, ctx, func(tx pgx.Tx) error {
		values, err := f.resource.AcquireTx(ctx, tx, b, []resources.Request{{ResourceID: ResourceID(ref.ID), Mode: mode, ExpectedVersion: version}}, 30*time.Second)
		if err != nil {
			return err
		}
		p = values[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if claim {
		err = f.tx(t, ctx, func(tx pgx.Tx) error { return f.resource.ClaimTx(ctx, tx, b, p) })
		if err != nil {
			t.Fatal(err)
		}
	}
	return workspaceProof(p)
}
func (f *adapterFixture) create(t *testing.T) (Ref, context.Context, resources.Binding) {
	t.Helper()
	ctx, b := f.worker(1)
	ref, err := Allocate(Binding{OwnerID: b.OwnerID, SessionKey: b.SessionKey, TaskID: b.TaskID, TaskRevision: 1, ConstraintsRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.tx(t, ctx, func(tx pgx.Tx) error { return f.resource.RegisterTx(ctx, tx, b, ResourceID(ref.ID)) }); err != nil {
		t.Fatal(err)
	}
	ctx = f.action(t, ctx, b, ref, "create", "", nil)
	p := f.acquire(t, ctx, b, ref, 1, resources.Write, true)
	if err = f.store.Create(ctx, ref, p); err != nil {
		t.Fatal(err)
	}
	return ref, ctx, b
}

func TestResourceWorkspaceCreateAndWriteConsumeExactClaimsIntegration(t *testing.T) {
	f := newAdapterFixture(t)
	ref, ctx, b := f.create(t)
	var version int64
	var inflight, state, evidence string
	if err := f.pool.QueryRow(ctx, `SELECT r.version,r.inflight_lease_id,l.state,l.evidence_id FROM execution_resources r JOIN execution_resource_leases l ON l.resource_id=r.resource_id WHERE r.resource_id=$1`, ResourceID(ref.ID)).Scan(&version, &inflight, &state, &evidence); err != nil || version != 2 || inflight != "" || state != "completed" || evidence == "" {
		t.Fatal("create claim not consumed", version, inflight, state, err)
	}
	ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("package main"))
	p := f.acquire(t, ctx, b, ref, 2, resources.Write, true)
	s, err := f.store.Write(ctx, ref, p, Snapshot{Path: "main.go"}, []byte("package main"))
	if err != nil || s.Version != 1 {
		t.Fatal(s, err)
	}
	if _, err = f.store.Write(ctx, ref, p, s, []byte("package main")); err == nil {
		t.Fatal("completed lease reused")
	}
	if err = f.pool.QueryRow(ctx, `SELECT version,inflight_lease_id FROM execution_resources WHERE resource_id=$1`, p.ResourceID).Scan(&version, &inflight); err != nil || version != 3 || inflight != "" {
		t.Fatal("write observation", version, inflight, err)
	}
	readCtx := f.action(t, ctx, b, ref, "read", "main.go", []byte("package main"))
	readProof := f.acquire(t, readCtx, b, ref, 3, resources.Read, false)
	read, err := f.store.Read(readCtx, ref, readProof, "main.go")
	if err != nil || read.Snapshot != s {
		t.Fatal(read.Snapshot, err)
	}
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_workspace_evidence`).Scan(&count); err != nil || count != 2 {
		t.Fatal("actual namespace/file evidence", count, err)
	}
	if _, err = NewResourceGuard(nil, f.admission, f.evidence); err == nil {
		t.Fatal("absent resource store permitted")
	}
	if _, err = NewResourceGuard(f.resource, nil, f.evidence); err == nil {
		t.Fatal("absent action/source proof permitted")
	}
	if _, err = NewResourceGuard(f.resource, f.admission, nil); err == nil {
		t.Fatal("absent effect observer permitted")
	}
	encoded, _ := json.Marshal(struct {
		Ref     Ref
		Proof   LeaseProof
		Binding resources.Binding
	}{ref, p, b})
	if string(encoded) == "" {
		t.Fatal("serialization")
	}
	var parsed map[string]any
	_ = json.Unmarshal(encoded, &parsed)
	if len(parsed["Proof"].(map[string]any)) != 0 || len(parsed["Binding"].(map[string]any)) != 0 {
		t.Fatal("private proof escaped JSON", string(encoded))
	}
}

func TestResourceWorkspaceRejectsHeldAndSameTransactionClaimsIntegration(t *testing.T) {
	for _, sameTX := range []bool{false, true} {
		t.Run(fmt.Sprint(sameTX), func(t *testing.T) {
			f := newAdapterFixture(t)
			ctx, b := f.worker(1)
			ref, _ := Allocate(Binding{OwnerID: b.OwnerID, SessionKey: b.SessionKey, TaskID: b.TaskID, TaskRevision: 1, ConstraintsRevision: 1})
			if err := f.tx(t, ctx, func(tx pgx.Tx) error { return f.resource.RegisterTx(ctx, tx, b, ResourceID(ref.ID)) }); err != nil {
				t.Fatal(err)
			}
			ctx = f.action(t, ctx, b, ref, "create", "", nil)
			p := f.acquire(t, ctx, b, ref, 1, resources.Write, false)
			f.admission.claimOnCheck = sameTX
			if err := f.store.Create(ctx, ref, p); err == nil {
				t.Fatal("uncommitted/held claim caused effect")
			}
			if _, err := os.Stat(filepath.Join(f.root, ref.ID)); !os.IsNotExist(err) {
				t.Fatal("namespace appeared before committed claim", err)
			}
			var state, inflight string
			if err := f.pool.QueryRow(ctx, `SELECT l.state,r.inflight_lease_id FROM execution_resource_leases l JOIN execution_resources r USING(resource_id) WHERE l.lease_id=$1`, p.LeaseID).Scan(&state, &inflight); err != nil || state != "held" || inflight != "" {
				t.Fatal("denial leaked claim", state, inflight, err)
			}
		})
	}
}

func TestResourceWorkspaceCurrentActionScopeAndSourcesIntegration(t *testing.T) {
	for _, change := range []string{"body", "path", "session", "owner", "task", "task_revision", "constraints_revision", "source", "terminal_run", "action"} {
		t.Run(change, func(t *testing.T) {
			f := newAdapterFixture(t)
			ref, ctx, b := f.create(t)
			ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("approved"))
			p := f.acquire(t, ctx, b, ref, 2, resources.Write, true)
			data := []byte("approved")
			expected := Snapshot{Path: "main.go"}
			switch change {
			case "body":
				data = []byte("tampered")
			case "path":
				expected.Path = "other.go"
			case "session":
				id := ctx.Value(resourceActionKey{})
				ctx, _ = f.worker(2)
				ctx = context.WithValue(ctx, resourceActionKey{}, id)
			case "owner":
				ref.Binding.OwnerID = "other-owner"
			case "task":
				ref.Binding.TaskID = "ws-t2"
				p.TaskID = "ws-t2"
			case "task_revision":
				_, _ = f.pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id='ws-t1'`)
			case "constraints_revision":
				_, _ = f.pool.Exec(ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id='ws-t1'`)
			case "source":
				_, _ = f.pool.Exec(ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, f.source.ID)
			case "terminal_run":
				_, _ = f.pool.Exec(ctx, `UPDATE runs SET status='completed',current_state='completed' WHERE run_id='ws-r1'`)
			case "action":
				id, _ := ctx.Value(resourceActionKey{}).(string)
				_, _ = f.pool.Exec(ctx, `UPDATE fixture_workspace_actions SET state='revoked' WHERE action_id=$1`, id)
			}
			if _, err := f.store.Write(ctx, ref, p, expected, data); err == nil {
				t.Fatal("stale action/source authority caused effect")
			}
			if _, err := os.Stat(filepath.Join(f.root, ref.ID, "tree", expected.Path)); !os.IsNotExist(err) {
				t.Fatal("denied bytes appeared", err)
			}
			var state string
			if err := f.pool.QueryRow(f.ctx, `SELECT state FROM execution_resource_leases WHERE lease_id=$1`, p.LeaseID).Scan(&state); err != nil || state != "claimed" {
				t.Fatal("denial invented lease completion", state, err)
			}
		})
	}
}

func TestResourceWorkspaceTwoContendersAndStaleObservationsIntegration(t *testing.T) {
	f := newAdapterFixture(t)
	ref, ctx, b := f.create(t)
	ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("one complete patch"))
	p := f.acquire(t, ctx, b, ref, 2, resources.Write, true)
	other, err := New(f.pool, f.root, f.guard, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	out := make(chan error, 2)
	for _, store := range []*Store{f.store, other} {
		go func(store *Store) {
			<-start
			_, err := store.Write(ctx, ref, p, Snapshot{Path: "main.go"}, []byte("one complete patch"))
			out <- err
		}(store)
	}
	close(start)
	success := 0
	denied := 0
	for range 2 {
		err := <-out
		if err == nil {
			success++
		} else if errors.Is(err, resources.ErrDenied) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal("double consumption/overwrite", success, denied)
	}
	if err = f.tx(t, ctx, func(tx pgx.Tx) error {
		_, err := f.resource.AcquireTx(ctx, tx, b, []resources.Request{{ResourceID: p.ResourceID, Mode: resources.Write, ExpectedVersion: 2}}, time.Second)
		return err
	}); !errors.Is(err, resources.ErrConflict) {
		t.Fatal("old resource observation revived", err)
	}
	ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("new patch"))
	fresh := f.acquire(t, ctx, b, ref, 3, resources.Write, true)
	if _, err = f.store.Write(ctx, ref, fresh, Snapshot{Path: "main.go"}, []byte("new patch")); !errors.Is(err, ErrConflict) {
		t.Fatal("old file observation revived", err)
	}
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_workspace_evidence WHERE kind='file_bytes'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("unexpected confirmed contenders", count, err)
	}
}

func TestResourceWorkspaceIndependentLeaseContendersIntegration(t *testing.T) {
	f := newAdapterFixture(t)
	ref, ctx, b := f.create(t)
	contexts := []context.Context{f.action(t, ctx, b, ref, "write", "main.go", []byte("patch-a")), f.action(t, ctx, b, ref, "write", "main.go", []byte("patch-b"))}
	type result struct {
		index int
		proof resources.Proof
		err   error
	}
	start := make(chan struct{})
	out := make(chan result, 2)
	for i, c := range contexts {
		go func(i int, c context.Context) {
			<-start
			var p resources.Proof
			err := f.tx(t, c, func(tx pgx.Tx) error {
				values, err := f.resource.AcquireTx(c, tx, b, []resources.Request{{ResourceID: ResourceID(ref.ID), Mode: resources.Write, ExpectedVersion: 2}}, time.Second*30)
				if err != nil {
					return err
				}
				p = values[0]
				return nil
			})
			out <- result{i, p, err}
		}(i, c)
	}
	close(start)
	var winner result
	granted, conflicts := 0, 0
	for range 2 {
		r := <-out
		if r.err == nil {
			winner = r
			granted++
		} else if errors.Is(r.err, resources.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(r.err)
		}
	}
	if granted != 1 || conflicts != 1 {
		t.Fatal("independent write leases overlapped", granted, conflicts)
	}
	c := contexts[winner.index]
	p := workspaceProof(winner.proof)
	if err := f.tx(t, c, func(tx pgx.Tx) error { return f.resource.ClaimTx(c, tx, b, winner.proof) }); err != nil {
		t.Fatal(err)
	}
	data := []byte([]string{"patch-a", "patch-b"}[winner.index])
	if _, err := f.store.Write(c, ref, p, Snapshot{Path: "main.go"}, data); err != nil {
		t.Fatal(err)
	}
	physical, err := os.ReadFile(filepath.Join(f.root, ref.ID, "tree", "main.go"))
	if err != nil || string(physical) != string(data) {
		t.Fatal("confirmed contender body", err)
	}
	var claims int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_resource_leases WHERE resource_id=$1 AND resource_version=2`, ResourceID(ref.ID)).Scan(&claims); err != nil || claims != 1 {
		t.Fatal("failed contender leaked lease", claims, err)
	}
}

func TestResourceWorkspaceUnknownClaimsSurviveFailureAndNeedOwnerEvidenceIntegration(t *testing.T) {
	for _, operation := range []string{"create", "write"} {
		t.Run(operation, func(t *testing.T) {
			f := newAdapterFixture(t)
			ctx, b := f.worker(1)
			var ref Ref
			var p LeaseProof
			version := int64(1)
			if operation == "create" {
				ref, _ = Allocate(Binding{OwnerID: b.OwnerID, SessionKey: b.SessionKey, TaskID: b.TaskID, TaskRevision: 1, ConstraintsRevision: 1})
				if err := f.tx(t, ctx, func(tx pgx.Tx) error { return f.resource.RegisterTx(ctx, tx, b, ResourceID(ref.ID)) }); err != nil {
					t.Fatal(err)
				}
				ctx = f.action(t, ctx, b, ref, "create", "", nil)
				p = f.acquire(t, ctx, b, ref, 1, resources.Write, true)
				f.evidence.failCreate = true
				if err := f.store.Create(ctx, ref, p); !errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("created namespace reported rollback", err)
				}
			} else {
				ref, ctx, b = f.create(t)
				version = 2
				ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("physical confirmed bytes"))
				p = f.acquire(t, ctx, b, ref, 2, resources.Write, true)
				f.evidence.failWrite = true
				if _, err := f.store.Write(ctx, ref, p, Snapshot{Path: "main.go"}, []byte("physical confirmed bytes")); !errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("post-rename failure reported rollback", err)
				}
				data, err := os.ReadFile(filepath.Join(f.root, ref.ID, "tree", "main.go"))
				if err != nil || string(data) != "physical confirmed bytes" {
					t.Fatal("actual effect absent", err)
				}
			}
			if _, err := os.Stat(filepath.Join(f.root, ref.ID, "private", "write-pending")); err != nil {
				t.Fatal("durable unknown marker lost", err)
			}
			var state, inflight string
			var actualVersion int64
			if err := f.pool.QueryRow(f.ctx, `SELECT l.state,r.inflight_lease_id,r.version FROM execution_resource_leases l JOIN execution_resources r USING(resource_id) WHERE l.lease_id=$1`, p.LeaseID).Scan(&state, &inflight, &actualVersion); err != nil || state != "claimed" || inflight != p.LeaseID || actualVersion != version {
				t.Fatal("unknown claim/effect erased", state, inflight, actualVersion, err)
			}
			var records int
			if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_workspace_evidence WHERE lease_id=$1`, p.LeaseID).Scan(&records); err != nil || records != 0 {
				t.Fatal("rolled-back evidence published", records, err)
			}
			// Simulate lease expiry without replacing the executor or granting proof.
			if _, err := f.pool.Exec(f.ctx, `UPDATE execution_resource_leases SET expires_at=clock_timestamp()-interval '1 microsecond' WHERE lease_id=$1`, p.LeaseID); err != nil {
				t.Fatal(err)
			}
			if err := f.tx(t, ctx, func(tx pgx.Tx) error {
				_, err := f.resource.AcquireTx(ctx, tx, b, []resources.Request{{ResourceID: p.ResourceID, Mode: resources.Write, ExpectedVersion: version}}, time.Second)
				return err
			}); !errors.Is(err, resources.ErrConflict) {
				t.Fatal("expired unknown allowed successor", err)
			}
			owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "ws-owner", Channel: "web", DeliveryID: "fixture-owner-control"})
			if err := f.tx(t, owner, func(tx pgx.Tx) error {
				_, err := f.resource.ReconcileTx(owner, tx, b.OwnerID, p.ResourceID, p.LeaseID, "unverified", version)
				return err
			}); !errors.Is(err, resources.ErrDenied) {
				t.Fatal("owner alone invented effect evidence", err)
			}
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO fixture_independent_reconciliation VALUES('independent-stop', $1,$2,$3,$4,true,true)`, b.OwnerID, p.ResourceID, p.LeaseID, version); err != nil {
				t.Fatal(err)
			}
			if err := f.tx(t, owner, func(tx pgx.Tx) error {
				_, err := f.resource.ReconcileTx(owner, tx, b.OwnerID, p.ResourceID, p.LeaseID, "independent-stop", version)
				return err
			}); !errors.Is(err, resources.ErrDenied) {
				t.Fatal("claimed stop overrode live executor", err)
			}
			if _, err := f.pool.Exec(f.ctx, `WITH stopped AS(UPDATE runs SET status='cancelled',current_state='cancelled' WHERE run_id=$1 RETURNING run_id) UPDATE session_ownership SET fence_epoch=fence_epoch+1 WHERE run_id IN(SELECT run_id FROM stopped)`, b.RunID); err != nil {
				t.Fatal(err)
			}
			if err := f.tx(t, owner, func(tx pgx.Tx) error {
				_, err := f.resource.ReconcileTx(owner, tx, b.OwnerID, p.ResourceID, p.LeaseID, "independent-stop", version)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			// Resource reconciliation does not rewrite file metadata, remove pending
			// markers or unblock the separately quarantined workspace automatically.
			var active bool
			err := f.pool.QueryRow(f.ctx, `SELECT state='active' FROM code_workspaces WHERE workspace_id=$1`, ref.ID).Scan(&active)
			if operation == "write" && (err != nil || active) {
				t.Fatal("resource reconciliation unblocked workspace", active, err)
			}
		})
	}
}
