package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This real-PG fixture supplies explicit same-TX live authority. It proves the
// workspace seam, not the separately owned production resource-lease adapter.
type fixtureGuard struct {
	entered          chan struct{}
	once             sync.Once
	denyConfirmation bool
}

func (g *fixtureGuard) CheckTx(ctx context.Context, tx pgx.Tx, a Access) error {
	var valid bool
	lock := " FOR SHARE"
	if a.Operation == "write" || a.Operation == "create" {
		lock = " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND lease_id=$7 AND fence_epoch=$8 AND resource_version=$9 AND run_id=$10 AND expires_at>clock_timestamp() FROM fixture_resource_authority WHERE resource_id=$1`+lock, a.Proof.ResourceID, a.Ref.Binding.OwnerID, a.Ref.Binding.SessionKey, a.Ref.Binding.TaskID, a.Ref.Binding.TaskRevision, a.Ref.Binding.ConstraintsRevision, a.Proof.LeaseID, a.Proof.FenceEpoch, a.Proof.ExpectedResourceVersion, a.Proof.RunID).Scan(&valid)
	if err != nil || !valid {
		return ErrDenied
	}
	if a.Operation == "write" && g.entered != nil {
		g.once.Do(func() { close(g.entered) })
	}
	return nil
}

// The fixture records an observation atomically with file metadata. It does not
// impersonate the production resource adapter or claim to consume a real lease.
func (g *fixtureGuard) ConfirmWriteTx(ctx context.Context, tx pgx.Tx, a Access, value Snapshot) error {
	if err := g.CheckTx(ctx, tx, a); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO fixture_confirmed_writes(resource_id,path,file_version,content_sha256,byte_length) VALUES($1,$2,$3,$4,$5)`, a.Proof.ResourceID, value.Path, value.Version, value.SHA256, value.Bytes)
	if err != nil {
		return err
	}
	if g.denyConfirmation {
		return ErrDenied
	}
	return nil
}

type workspaceFixture struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	store *Store
	root  string
	guard *fixtureGuard
}

func newFixture(t *testing.T) *workspaceFixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated BUTLER_TEST_POSTGRES_URL required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated *_test PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	if _, err = rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	schema := "workspace_fixture_" + hex.EncodeToString(id[:])
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "060_isolated_code_workspaces.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE fixture_resource_authority(resource_id text PRIMARY KEY,owner_id text,session_key text,task_id text,task_revision bigint,constraints_revision bigint,lease_id text,fence_epoch bigint,resource_version bigint,run_id text,expires_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE fixture_confirmed_writes(resource_id text,path text,file_version bigint,content_sha256 text,byte_length bigint,PRIMARY KEY(resource_id,path,file_version))`); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	guard := &fixtureGuard{}
	store, err := New(pool, root, guard, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &workspaceFixture{ctx, pool, store, root, guard}
}
func (f *workspaceFixture) create(t *testing.T, binding Binding) (Ref, LeaseProof) {
	t.Helper()
	ref, err := Allocate(binding)
	if err != nil {
		t.Fatal(err)
	}
	proof := LeaseProof{LeaseID: "private-lease-" + ref.ID, ResourceID: ResourceID(ref.ID), FenceEpoch: 1, ExpectedResourceVersion: 1, RunID: "run-" + binding.TaskID, SessionKey: binding.SessionKey, TaskID: binding.TaskID}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO fixture_resource_authority VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,clock_timestamp()+interval '1 minute')`, proof.ResourceID, binding.OwnerID, binding.SessionKey, binding.TaskID, binding.TaskRevision, binding.ConstraintsRevision, proof.LeaseID, proof.FenceEpoch, proof.ExpectedResourceVersion, proof.RunID); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Create(f.ctx, ref, proof); err != nil {
		t.Fatal(err)
	}
	return ref, proof
}
func testBinding(owner, session, task string) Binding { return Binding{owner, session, task, 1, 1} }
func (f *workspaceFixture) seed(t *testing.T, ref Ref, proof LeaseProof, name, body string) Snapshot {
	t.Helper()
	value, err := f.store.Write(f.ctx, ref, proof, Snapshot{Path: name}, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestConcurrentRealPatchesCannotOverwriteAcrossStoreInstancesIntegration(t *testing.T) {
	f := newFixture(t)
	ref, proof := f.create(t, testBinding("owner", "web:owner", "task"))
	old := f.seed(t, ref, proof, "src/main.go", "original")
	second, err := New(f.pool, f.root, f.guard, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, store := range []*Store{f.store, second} {
		go func(i int, store *Store) {
			<-start
			_, err := store.Write(f.ctx, ref, proof, old, []byte([]string{"patch-one", "patch-two"}[i]))
			results <- err
		}(i, store)
	}
	close(start)
	var succeeded, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatal("concurrent overwrite", succeeded, conflicts)
	}
	current, err := f.store.Read(f.ctx, ref, proof, old.Path)
	if err != nil || current.Version != 2 || (string(current.Content) != "patch-one" && string(current.Content) != "patch-two") {
		t.Fatal("not one complete patch", current.Snapshot, err)
	}
	if _, err = f.store.Write(f.ctx, ref, proof, old, []byte("stale")); !errors.Is(err, ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	before := current.Snapshot
	reverted, err := f.store.Write(f.ctx, ref, proof, before, []byte("original"))
	if err != nil || reverted.SHA256 != old.SHA256 || reverted.Version != 3 {
		t.Fatal("version did not distinguish ABA", reverted, err)
	}
	if _, err = f.store.Write(f.ctx, ref, proof, old, []byte("old plan")); !errors.Is(err, ErrConflict) {
		t.Fatal("same old hash erased version conflict", err)
	}
	var confirmations int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_confirmed_writes WHERE resource_id=$1`, proof.ResourceID).Scan(&confirmations); err != nil || confirmations != 3 {
		t.Fatal("confirmation not atomic/exactly once with successful file versions", confirmations, err)
	}
}
func TestIsolatedCopiesScopesAndPrivateTempsIntegration(t *testing.T) {
	f := newFixture(t)
	a, pa := f.create(t, testBinding("owner", "web:owner", "task-a"))
	b, pb := f.create(t, testBinding("owner", "web:owner", "task-b"))
	c, pc := f.create(t, testBinding("other", "web:other", "task-c"))
	sa := f.seed(t, a, pa, "main.go", "same-base")
	f.seed(t, b, pb, "main.go", "same-base")
	f.seed(t, c, pc, "main.go", "other-owner")
	if _, err := f.store.Write(f.ctx, a, pa, sa, []byte("branch-a")); err != nil {
		t.Fatal(err)
	}
	read, err := f.store.Read(f.ctx, b, pb, "main.go")
	if err != nil || string(read.Content) != "same-base" {
		t.Fatal("branch files shared", err)
	}
	currentA, err := f.store.Read(f.ctx, a, pa, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	parallel := make(chan error, 2)
	start := make(chan struct{})
	for _, item := range []struct {
		ref      Ref
		proof    LeaseProof
		expected Snapshot
	}{{a, pa, currentA.Snapshot}, {b, pb, read.Snapshot}} {
		go func(item struct {
			ref      Ref
			proof    LeaseProof
			expected Snapshot
		}) {
			<-start
			_, err := f.store.Write(f.ctx, item.ref, item.proof, item.expected, []byte("independent-"+item.ref.ID))
			parallel <- err
		}(item)
	}
	close(start)
	for range 2 {
		if err := <-parallel; err != nil {
			t.Fatal("independent task work blocked", err)
		}
	}
	forged := b
	forged.Binding = a.Binding
	if _, err = f.store.Read(f.ctx, forged, pa, "main.go"); !errors.Is(err, ErrDenied) {
		t.Fatal("task/lease crossed workspace", err)
	}
	forged = a
	forged.Binding = c.Binding
	if _, err = f.store.Read(f.ctx, forged, pc, "main.go"); !errors.Is(err, ErrDenied) {
		t.Fatal("owner/session crossed workspace", err)
	}
	for _, ref := range []Ref{a, b, c} {
		for _, part := range []string{"", "private", "private/tmp"} {
			info, err := os.Stat(filepath.Join(f.root, ref.ID, part))
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatal("private directory mode", err)
			}
		}
		entries, err := os.ReadDir(filepath.Join(f.root, ref.ID, "private", "tmp"))
		if err != nil || len(entries) != 0 {
			t.Fatal("patch temp retained/shared", err)
		}
	}
	for _, name := range []string{"../" + a.ID + "/main.go", "private/tmp/guess", "tmp/other", ".env", ".git/config", "credentials.json"} {
		if _, err = f.store.Read(f.ctx, b, pb, name); !errors.Is(err, ErrDenied) {
			t.Fatal("private path admitted", name, err)
		}
	}
}
func TestExpiredOrSupersededAuthorityCannotWriteAfterWorkspaceWaitIntegration(t *testing.T) {
	f := newFixture(t)
	ref, proof := f.create(t, testBinding("owner", "web:owner", "task"))
	old := f.seed(t, ref, proof, "main.go", "original")
	lock, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(lock)
	if _, err = lock.Exec(f.ctx, `SELECT workspace_id FROM code_workspaces WHERE workspace_id=$1 FOR UPDATE`, ref.ID); err != nil {
		t.Fatal(err)
	}
	// The other connection waits on this workspace while holding its first live
	// authority check. Expiry advances by PG clock without lock-order inversion.
	if _, err = f.pool.Exec(f.ctx, `UPDATE fixture_resource_authority SET expires_at=clock_timestamp()+interval '100 milliseconds' WHERE resource_id=$1`, proof.ResourceID); err != nil {
		t.Fatal(err)
	}
	f.guard.entered = make(chan struct{})
	result := make(chan error, 1)
	go func() { _, err := f.store.Write(f.ctx, ref, proof, old, []byte("late")); result <- err }()
	select {
	case <-f.guard.entered:
	case <-f.ctx.Done():
		t.Fatal("writer did not enter authority check")
	}
	time.Sleep(150 * time.Millisecond)
	if err = lock.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrDenied) {
		t.Fatal("expired writer wrote after lock wait", err)
	}
	actual, err := os.ReadFile(filepath.Join(f.root, ref.ID, "tree", "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("late effect", err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE fixture_resource_authority SET expires_at=clock_timestamp()+interval '1 minute',fence_epoch=2 WHERE resource_id=$1`, proof.ResourceID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Write(f.ctx, ref, proof, old, []byte("superseded")); !errors.Is(err, ErrDenied) {
		t.Fatal("superseded resource lease admitted", err)
	}
}
func TestPhysicalDriftAndFailedMetadataCommitBlockWithoutReplayIntegration(t *testing.T) {
	for _, mode := range []string{"external-edit", "metadata-failure", "confirmation-failure", "durable-pending-marker"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			ref, proof := f.create(t, testBinding("owner", "web:owner", "task"))
			old := f.seed(t, ref, proof, "main.go", "original")
			if mode == "external-edit" {
				if err := os.WriteFile(filepath.Join(f.root, ref.ID, "tree", "main.go"), []byte("manual-owner-edit"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.Write(f.ctx, ref, proof, old, []byte("overwrite")); !errors.Is(err, ErrDrift) {
					t.Fatal("external change overwritten", err)
				}
			} else if mode == "metadata-failure" {
				if _, err := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_fixture_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture durable write failed'; END $$; CREATE TRIGGER reject_fixture_file BEFORE INSERT OR UPDATE ON code_workspace_files FOR EACH ROW EXECUTE FUNCTION reject_fixture_file()`); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.Write(f.ctx, ref, proof, old, []byte("physical-patch")); !errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("uncertain write called success", err)
				}
			} else if mode == "confirmation-failure" {
				f.guard.denyConfirmation = true
				if _, err := f.store.Write(f.ctx, ref, proof, old, []byte("physical-patch")); !errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("unconfirmed resource write called success", err)
				}
			} else {
				// A surviving checkpoint must block even matching metadata after an
				// unknown acknowledgement; hash equality cannot settle authority.
				parent, err := f.store.root.OpenRoot(ref.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = markPending(parent, "opaque-uncertain-attempt"); err != nil {
					t.Fatal(err)
				}
				parent.Close()
				if _, err = f.store.Read(f.ctx, ref, proof, "main.go"); !errors.Is(err, ErrDrift) {
					t.Fatal("pending matching metadata released bytes", err)
				}
			}
			if mode == "metadata-failure" || mode == "confirmation-failure" {
				var version int64
				var confirmations int
				if err := f.pool.QueryRow(f.ctx, `SELECT version FROM code_workspace_files WHERE workspace_id=$1 AND path=$2`, ref.ID, old.Path).Scan(&version); err != nil || version != old.Version {
					t.Fatal("failed confirmation committed file metadata", version, err)
				}
				if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_confirmed_writes WHERE resource_id=$1`, proof.ResourceID).Scan(&confirmations); err != nil || confirmations != 1 {
					t.Fatal("failed confirmation committed resource observation", confirmations, err)
				}
				if _, err := os.Lstat(filepath.Join(f.root, ref.ID, "private", "write-pending")); err != nil {
					t.Fatal("uncertain physical effect lost durable checkpoint", err)
				}
			}
			var state, reason string
			if err := f.pool.QueryRow(f.ctx, `SELECT state,blocked_reason FROM code_workspaces WHERE workspace_id=$1`, ref.ID).Scan(&state, &reason); err != nil || state != "blocked" || reason == "" {
				t.Fatal("workspace not quarantined", state, reason, err)
			}
			restarted, err := New(f.pool, f.root, f.guard, DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			if _, err = restarted.Write(f.ctx, ref, proof, old, []byte("automatic-replay")); !errors.Is(err, ErrDrift) {
				t.Fatal("restart cleared block", err)
			}
			if _, err = restarted.Read(f.ctx, ref, proof, "main.go"); !errors.Is(err, ErrDrift) {
				t.Fatal("uncertain bytes released", err)
			}
			actual, err := os.ReadFile(filepath.Join(f.root, ref.ID, "tree", "main.go"))
			if err != nil || string(actual) == "overwrite" || string(actual) == "automatic-replay" || (mode != "durable-pending-marker" && string(actual) == "original") {
				t.Fatal("invented rollback/replay", err)
			}
		})
	}
}
func TestPathsSymlinksHardlinksAndScopeEscapesDeniedIntegration(t *testing.T) {
	for _, kind := range []string{"paths", "symlink", "hardlink", "private-temp-alias"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			ref, proof := f.create(t, testBinding("owner", "web:owner", "task"))
			old := f.seed(t, ref, proof, "main.go", "original")
			switch kind {
			case "paths":
				for _, name := range []string{"/etc/passwd", "../main.go", "./main.go", "main.go/", "x\\main.go", "C:/secret", ".git/config", "PRIVATE/token", "nested/.env.production", "secrets/token", "credentials.json", "id_rsa", "cert/private.key", "token.json", ".docker/config.json"} {
					if _, err := f.store.Write(f.ctx, ref, proof, Snapshot{Path: name}, []byte("x")); !errors.Is(err, ErrDenied) {
						t.Fatal("path admitted", name, err)
					}
				}
			case "symlink", "hardlink":
				path := filepath.Join(f.root, ref.ID, "tree", "main.go")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(t.TempDir(), "private.txt")
				if err := os.WriteFile(other, []byte("outside-private"), 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(other, path)
				} else {
					err = os.Link(other, path)
				}
				if err != nil {
					t.Fatal(err)
				}
				if data, err := f.store.Read(f.ctx, ref, proof, "main.go"); !errors.Is(err, ErrDenied) || len(data.Content) != 0 {
					t.Fatal("external alias released data", err)
				}
			case "private-temp-alias":
				path := filepath.Join(f.root, ref.ID, "private", "tmp")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(f.root, ref.ID, "tree"), path); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.Write(f.ctx, ref, proof, old, []byte("new")); !errors.Is(err, ErrDenied) {
					t.Fatal("temp directory alias admitted", err)
				}
			}
		})
	}
}
func TestWorkspaceLimitsAndMigrationHistoryCannotBeDiscardedIntegration(t *testing.T) {
	f := newFixture(t)
	limited, err := New(f.pool, f.root, f.guard, Limits{4, 6, 2})
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	if _, err = New(f.pool, f.root, nil, DefaultLimits()); !errors.Is(err, ErrDenied) {
		t.Fatal("nil authority enabled workspace", err)
	}
	f.store = limited
	ref, proof := f.create(t, testBinding("owner", "web:owner", "task"))
	a := f.seed(t, ref, proof, "a.go", "1234")
	f.seed(t, ref, proof, "b.go", "12")
	for _, attempt := range []struct {
		expected Snapshot
		body     string
	}{{a, "12345"}, {Snapshot{Path: "c.go"}, "x"}, {Snapshot{Path: "b.go", Version: 1, SHA256: digest([]byte("12")), Bytes: 2}, "123"}} {
		if _, err := limited.Write(f.ctx, ref, proof, attempt.expected, []byte(attempt.body)); !errors.Is(err, ErrDenied) {
			t.Fatal("limit widened", err)
		}
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "060_isolated_code_workspaces.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, string(down)); err == nil {
		t.Fatal("downgrade erased populated workspace history")
	}
}
