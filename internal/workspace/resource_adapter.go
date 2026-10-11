package workspace

import (
	"context"
	"encoding/hex"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
)

// ResourceAdmission owns exact action/source/budget policy. It derives the
// private live binding, checks approved path/body hash/source versions before
// every effect/read, and joins this TX without network or another connection.
// It must not claim a resource in this transaction or mint model authority.
type ResourceAdmission interface {
	CheckWorkspaceTx(context.Context, pgx.Tx, Access) (resources.Binding, error)
}

// ResourceEvidence independently observes fixed operator-provisioned storage
// and records exact namespace/file effect evidence on this same transaction.
// It must never trust a model snapshot or supply a blanket success verdict.
type ResourceEvidence interface {
	RecordNamespaceTx(context.Context, pgx.Tx, Access) (string, error)
	RecordWriteTx(context.Context, pgx.Tx, Access, Snapshot) (string, error)
}

type ResourceGuard struct {
	resources *resources.Store
	admission ResourceAdmission
	evidence  ResourceEvidence
}

func NewResourceGuard(store *resources.Store, admission ResourceAdmission, evidence ResourceEvidence) (*ResourceGuard, error) {
	if store == nil || admission == nil || evidence == nil {
		return nil, ErrDenied
	}
	return &ResourceGuard{store, admission, evidence}, nil
}

func resourceProof(p LeaseProof) resources.Proof {
	return resources.Proof{LeaseID: p.LeaseID, ResourceID: p.ResourceID, FenceEpoch: p.FenceEpoch, ExpectedResourceVersion: p.ExpectedResourceVersion, RunID: p.RunID, SessionKey: p.SessionKey, TaskID: p.TaskID}
}

func (g *ResourceGuard) check(ctx context.Context, tx pgx.Tx, a Access) (resources.Binding, error) {
	var empty resources.Binding
	if g == nil || g.resources == nil || g.admission == nil || g.evidence == nil {
		return empty, ErrDenied
	}
	if _, err := access(a.Ref, a.Proof, a.Operation, a.Path, a.Bytes); err != nil {
		return empty, err
	}
	mode := resources.Read
	switch a.Operation {
	case "create":
		if a.Path != "" || a.Bytes != 0 || a.ContentSHA256 != "" {
			return empty, ErrDenied
		}
		mode = resources.Write
	case "write":
		if !validPath(a.Path) || a.Bytes < 0 || !validDigest(a.ContentSHA256) {
			return empty, ErrDenied
		}
		mode = resources.Write
	case "read":
		if !validPath(a.Path) || a.Bytes < 0 || (a.ContentSHA256 != "" && !validDigest(a.ContentSHA256)) {
			return empty, ErrDenied
		}
	default:
		return empty, ErrDenied
	}
	b, err := g.admission.CheckWorkspaceTx(ctx, tx, a)
	if err != nil {
		return empty, err
	}
	t, ok := ownership.TokenFromContext(ctx)
	r := a.Ref.Binding
	if !ok || b.OwnerID != r.OwnerID || b.SessionKey != r.SessionKey || b.TaskID != r.TaskID || b.TaskRevision != r.TaskRevision || b.ConstraintsRevision != r.ConstraintsRevision || b.RunID != a.Proof.RunID || b.SessionKey != t.SessionKey || b.RunID != t.RunID || b.LeaseEpoch != t.FenceEpoch {
		return empty, ErrDenied
	}
	if err = g.resources.CheckTx(ctx, tx, b, resourceProof(a.Proof), mode); err != nil {
		return empty, err
	}
	if mode == resources.Write {
		var committed bool
		// A committed claim is visible from a preceding transaction. Reject even
		// an accidental authority callback which creates a new claim on this TX.
		err = tx.QueryRow(ctx, `SELECT l.state='claimed' AND r.inflight_lease_id=l.lease_id AND l.xmin<>pg_current_xact_id()::xid FROM execution_resource_leases l JOIN execution_resources r USING(resource_id) WHERE l.lease_id=$1 AND r.resource_id=$2`, a.Proof.LeaseID, a.Proof.ResourceID).Scan(&committed)
		if err != nil {
			return empty, err
		}
		if !committed {
			return empty, resources.ErrConflict
		}
	}
	return b, nil
}

func (g *ResourceGuard) CheckTx(ctx context.Context, tx pgx.Tx, a Access) error {
	_, err := g.check(ctx, tx, a)
	return err
}

func (g *ResourceGuard) ConfirmCreateTx(ctx context.Context, tx pgx.Tx, a Access) error {
	if a.Operation != "create" {
		return ErrDenied
	}
	if _, err := g.check(ctx, tx, a); err != nil {
		return err
	}
	var valid bool
	r := a.Ref.Binding
	err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND state='active' AND generation=1 AND NOT EXISTS(SELECT 1 FROM code_workspace_files f WHERE f.workspace_id=w.workspace_id) FROM code_workspaces w WHERE workspace_id=$1`, a.Ref.ID, r.OwnerID, r.SessionKey, r.TaskID, r.TaskRevision, r.ConstraintsRevision).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrDenied
	}
	evidence, err := g.evidence.RecordNamespaceTx(ctx, tx, a)
	if err != nil {
		return err
	}
	b, err := g.check(ctx, tx, a)
	if err != nil {
		return err
	}
	_, err = g.resources.CompleteTx(ctx, tx, b, resourceProof(a.Proof), evidence)
	return err
}

func (g *ResourceGuard) ConfirmWriteTx(ctx context.Context, tx pgx.Tx, a Access, s Snapshot) error {
	if a.Operation != "write" || s.Path != a.Path || s.SHA256 != a.ContentSHA256 || s.Bytes != a.Bytes || s.Version < 1 {
		return ErrDenied
	}
	if _, err := g.check(ctx, tx, a); err != nil {
		return err
	}
	actual, err := snapshot(ctx, tx, a.Ref.ID, a.Path)
	if err != nil {
		return err
	}
	if actual != s {
		return ErrConflict
	}
	r := a.Ref.Binding
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND state='active' FROM code_workspaces WHERE workspace_id=$1`, a.Ref.ID, r.OwnerID, r.SessionKey, r.TaskID, r.TaskRevision, r.ConstraintsRevision).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrDenied
	}
	evidence, err := g.evidence.RecordWriteTx(ctx, tx, a, s)
	if err != nil {
		return err
	}
	b, err := g.check(ctx, tx, a)
	if err != nil {
		return err
	}
	_, err = g.resources.CompleteTx(ctx, tx, b, resourceProof(a.Proof), evidence)
	return err
}

func validDigest(s string) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == s
}
