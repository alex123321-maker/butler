package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
)

// The mutation happens after metadata CAS and before independent confirmation,
// exercising the real filesystem/PG commit gap with a committed resource claim.
func TestWorkspaceObserverRejectsPhysicalConfirmationDriftIntegration(t *testing.T) {
	for _, operation := range []string{"create", "write"} {
		cases := []struct {
			name   string
			change func(string) error
		}{
			{"unexpected_namespace_entry", func(root string) error { return os.WriteFile(filepath.Join(root, "stray"), []byte("fixture"), 0600) }},
			{"unexpected_private_entry", func(root string) error {
				return os.WriteFile(filepath.Join(root, "private", "stray"), []byte("fixture"), 0600)
			}},
			{"shared_temp_residue", func(root string) error {
				return os.WriteFile(filepath.Join(root, "private", "tmp", "stray"), []byte("fixture"), 0600)
			}},
			{"shared_private_permissions", func(root string) error { return os.Chmod(filepath.Join(root, "private"), 0755) }},
			{"shared_tree_permissions", func(root string) error { return os.Chmod(filepath.Join(root, "tree"), 0755) }},
			{"missing_checkpoint", func(root string) error { return os.Remove(filepath.Join(root, "private", "write-pending")) }},
			{"invalid_checkpoint", func(root string) error {
				return os.WriteFile(filepath.Join(root, "private", "write-pending"), []byte("untrusted"), 0600)
			}},
			{"shared_checkpoint", func(root string) error { return os.Chmod(filepath.Join(root, "private", "write-pending"), 0644) }},
			{"body_changed", func(root string) error {
				return os.WriteFile(filepath.Join(root, "tree", "main.go"), []byte("modified fixture"), 0600)
			}},
		}
		if operation == "write" {
			cases = append(cases, struct {
				name   string
				change func(string) error
			}{"shared_file_permissions", func(root string) error { return os.Chmod(filepath.Join(root, "tree", "main.go"), 0644) }})
		}
		for _, tc := range cases {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				f := newAdapterFixture(t)
				var ref Ref
				var err error
				ctx, b := f.worker(1)
				var p LeaseProof
				if operation == "create" {
					ref, err = Allocate(Binding{b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision})
					if err != nil {
						t.Fatal(err)
					}
					if err = f.tx(t, ctx, func(tx pgx.Tx) error { return f.resource.RegisterTx(ctx, tx, b, ResourceID(ref.ID)) }); err != nil {
						t.Fatal(err)
					}
					ctx = f.action(t, ctx, b, ref, "create", "", nil)
					p = f.acquire(t, ctx, b, ref, 1, resources.Write, true)
				} else {
					ref, ctx, b = f.create(t)
					ctx = f.action(t, ctx, b, ref, "write", "main.go", []byte("package main"))
					p = f.acquire(t, ctx, b, ref, 2, resources.Write, true)
				}
				f.evidence.before = func(context.Context, pgx.Tx, Access) error { return tc.change(filepath.Join(f.root, ref.ID)) }
				if operation == "create" {
					err = f.store.Create(ctx, ref, p)
				} else {
					_, err = f.store.Write(ctx, ref, p, Snapshot{Path: "main.go"}, []byte("package main"))
				}
				if !errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("false confirmation", err)
				}
				var completed, evidence, files int
				if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_resource_leases WHERE lease_id=$1 AND state='completed'),(SELECT count(*) FROM fixture_workspace_evidence WHERE lease_id=$1),(SELECT count(*) FROM code_workspace_files WHERE workspace_id=$2)`, p.LeaseID, ref.ID).Scan(&completed, &evidence, &files); err != nil || completed != 0 || evidence != 0 || files != 0 {
					t.Fatal("uncertain effect published", completed, evidence, files, err)
				}
				var state string
				err = f.pool.QueryRow(ctx, `SELECT state FROM code_workspaces WHERE workspace_id=$1`, ref.ID).Scan(&state)
				if operation == "create" {
					if !errors.Is(err, pgx.ErrNoRows) {
						t.Fatal("failed namespace metadata published", state, err)
					}
				} else if err != nil || state != "blocked" {
					t.Fatal("uncertain copy reusable", state, err)
				}
			})
		}
	}
}

func TestWorkspaceObserverUsesCurrentBindingAndActualMetadataIntegration(t *testing.T) {
	f := newAdapterFixture(t)
	ref, ctx, b := f.create(t)
	body := []byte("package main // observer fixture")
	ctx = f.action(t, ctx, b, ref, "write", "main.go", body)
	p := f.acquire(t, ctx, b, ref, 2, resources.Write, true)
	var observed Observation
	f.evidence.before = func(ctx context.Context, tx pgx.Tx, a Access) error {
		for _, change := range []func(*Access){
			func(a *Access) { a.Ref.Binding.OwnerID = "foreign" },
			func(a *Access) { a.Ref.Binding.SessionKey = "ws-s2" },
			func(a *Access) { a.Ref.Binding.TaskRevision++ },
			func(a *Access) { a.Ref.Binding.ConstraintsRevision++ },
			func(a *Access) { a.ContentSHA256 = digest([]byte("wrong")) },
			func(a *Access) { a.Bytes++ },
			func(a *Access) { a.Path = "private/write-pending" },
			func(a *Access) { a.Operation = "read" },
		} {
			other := a
			change(&other)
			if _, err := f.evidence.observer.FileTx(ctx, tx, other); err == nil {
				t.Fatal("foreign binding/intent observed", other.Operation)
			}
		}
		var err error
		observed, err = f.evidence.observer.FileTx(ctx, tx, a)
		return err
	}
	s, err := f.store.Write(ctx, ref, p, Snapshot{Path: "main.go"}, body)
	if err != nil || observed.Snapshot != s || observed.Kind != "file_bytes" || observed.Digest != observation(ref.ID, "file_bytes", s).Digest {
		t.Fatal("actual independent observation", observed, s, err)
	}
	raw, err := json.Marshal(observed)
	if err != nil || strings.Contains(string(raw), string(body)) || strings.Contains(string(raw), f.root) || strings.Contains(string(raw), b.OwnerID) || strings.Contains(string(raw), p.LeaseID) {
		t.Fatal("private observation material escaped")
	}
	var stored string
	if err = f.pool.QueryRow(ctx, `SELECT observation_hash FROM fixture_workspace_evidence WHERE lease_id=$1`, p.LeaseID).Scan(&stored); err != nil || stored != observed.Digest {
		t.Fatal("actual digest not committed", err)
	}
}
