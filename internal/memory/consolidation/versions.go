package consolidation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type Head struct {
	ViewKey  string `json:"view_key"`
	Revision int64  `json:"revision"`
	ActiveID string `json:"active_source_id"`
}

type Generation struct {
	Source          catalog.SourceRef   `json:"source"`
	InputID         string              `json:"input_source_id"`
	PolicyID        string              `json:"policy_id"`
	ViewKey         string              `json:"view_key"`
	Algorithm       string              `json:"algorithm"`
	OperationID     string              `json:"operation_id"`
	Parents         []catalog.SourceRef `json:"parents"`
	UnitIDs         []string            `json:"unit_ids"`
	BaseID          string              `json:"base_source_id"`
	BaseRevision    int64               `json:"base_revision"`
	Diff            Diff                `json:"diff"`
	CreatedAt       time.Time           `json:"created_at"`
	Provider        string              `json:"provider"`
	Model           string              `json:"model"`
	Quote           json.RawMessage     `json:"reserved_quote"`
	Usage           json.RawMessage     `json:"observed_usage"`
	Retired         bool                `json:"retired"`
	CleanupComplete bool                `json:"cleanup_complete"`
}

type Preview struct {
	Generation     Generation     `json:"generation"`
	Head           Head           `json:"current_head"`
	Diff           Diff           `json:"current_diff"`
	ModelGroups    Selection      `json:"model_groups"`
	Evidence       Envelope       `json:"exact_evidence"`
	Representation Representation `json:"data_representation"`
	Status         string         `json:"catalog_status"`
}

type Activation struct {
	SourceID         string `json:"source_id"`
	ExpectedActiveID string `json:"expected_active_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func readHead(ctx context.Context, tx pgx.Tx, owner, key string, lock bool) (Head, error) {
	h := Head{ViewKey: key}
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, `SELECT revision,COALESCE(active_source_id,'') FROM memory_consolidation_heads WHERE owner_id=$1 AND view_key=$2`+clause, owner, key).Scan(&h.Revision, &h.ActiveID)
	if err == pgx.ErrNoRows {
		return h, nil
	}
	return h, err
}

func readGeneration(ctx context.Context, tx pgx.Tx, owner, id string) (Generation, error) {
	var g Generation
	var parents, units, diff []byte
	err := tx.QueryRow(ctx, `SELECT g.source_id,g.source_revision,g.content_hash,g.input_source_id,i.policy_id,g.view_key,g.algorithm,g.operation_id,g.parent_manifest,g.unit_ids,g.base_source_id,g.base_revision,g.diff,g.created_at,a.provider,a.model,a.quote,a.usage,i.retired,i.cleanup_complete FROM memory_consolidation_generations g JOIN memory_consolidation_inputs i ON i.source_id=g.input_source_id JOIN memory_model_attempts a ON a.operation_id=g.operation_id WHERE g.source_id=$1 AND g.owner_id=$2 AND i.owner_id=$2`, id, owner).Scan(&g.Source.ID, &g.Source.Revision, &g.Source.Hash, &g.InputID, &g.PolicyID, &g.ViewKey, &g.Algorithm, &g.OperationID, &parents, &units, &g.BaseID, &g.BaseRevision, &diff, &g.CreatedAt, &g.Provider, &g.Model, &g.Quote, &g.Usage, &g.Retired, &g.CleanupComplete)
	if err != nil {
		if err == pgx.ErrNoRows {
			return g, catalog.ErrDenied
		}
		return g, err
	}
	if json.Unmarshal(parents, &g.Parents) != nil || json.Unmarshal(units, &g.UnitIDs) != nil || json.Unmarshal(diff, &g.Diff) != nil || g.Algorithm != Version {
		return g, catalog.ErrInvalid
	}
	return g, nil
}

func (s *Store) currentContexts(ctx context.Context, tx pgx.Tx, g Generation) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT policy FROM memory_model_policies WHERE policy_id=$1`, g.PolicyID).Scan(&raw); err != nil {
		return err
	}
	var p jobs.ModelPolicy
	if json.Unmarshal(raw, &p) != nil || p.Handler != Version || p.MaxJobs != 1 || len(p.ContextPins) < 1 || len(p.ContextPins) > 8 {
		return jobs.ErrClaim
	}
	pins, err := s.Jobs.PrepareConsolidationContextsTx(ctx, tx, p.ModelPolicyRequest)
	if err != nil {
		return err
	}
	if string(mustJSON(pins)) != string(mustJSON(p.ContextPins)) {
		return catalog.ErrStale
	}
	return nil
}

func readPriorIDs(ctx context.Context, tx pgx.Tx, owner string, head Head) ([]string, error) {
	ids := []string{}
	if head.ActiveID == "" {
		return ids, nil
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT unit_ids FROM memory_consolidation_generations WHERE source_id=$1 AND owner_id=$2`, head.ActiveID, owner).Scan(&raw); err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &ids) != nil {
		return nil, catalog.ErrInvalid
	}
	return ids, nil
}

func (s *Store) Preview(ctx context.Context, id string) (Preview, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil || s.Jobs == nil {
		return Preview{}, catalog.ErrDenied
	}
	tx, err := s.Catalog.Pool().Begin(ctx)
	if err != nil {
		return Preview{}, err
	}
	defer tx.Rollback(ctx)
	g, err := readGeneration(ctx, tx, actor.OwnerID, id)
	if err != nil {
		return Preview{}, err
	}
	if g.Retired {
		return Preview{}, catalog.ErrDeleted
	}
	if err = s.currentContexts(ctx, tx, g); err != nil {
		return Preview{}, err
	}
	root, err := s.Catalog.ReviewDerivedTx(ctx, tx, g.Source)
	if err != nil {
		return Preview{}, err
	}
	var result Representation
	if strict(root.Body, &result) != nil {
		return Preview{}, catalog.ErrInvalid
	}
	input, err := s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{{ID: g.InputID}}, "memory_read")
	if err != nil || len(input) != 1 {
		return Preview{}, catalog.ErrStale
	}
	envelope, err := readEnvelope(input)
	if err != nil {
		return Preview{}, err
	}
	h, err := readHead(ctx, tx, actor.OwnerID, g.ViewKey, false)
	if err != nil {
		return Preview{}, err
	}
	before, err := readPriorIDs(ctx, tx, actor.OwnerID, h)
	if err != nil {
		return Preview{}, err
	}
	g.Source.AccessRevision = root.AccessRevision
	view := Preview{Generation: g, Head: h, Diff: compareIDs(before, result), ModelGroups: selectionOf(result), Evidence: envelope, Representation: result, Status: root.Status}
	return view, tx.Rollback(ctx)
}

// Activate is also the rollback primitive. Current context/source/temporal
// proof and both expected head ID and revision prevent stale or ABA replacement.
func (s *Store) Activate(ctx context.Context, q Activation) (Head, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil || s.Jobs == nil {
		return Head{}, catalog.ErrDenied
	}
	if q.SourceID == "" || q.ExpectedRevision < 0 {
		return Head{}, catalog.ErrInvalid
	}
	tx, err := s.Catalog.Pool().Begin(ctx)
	if err != nil {
		return Head{}, err
	}
	defer tx.Rollback(ctx)
	g, err := readGeneration(ctx, tx, actor.OwnerID, q.SourceID)
	if err != nil {
		return Head{}, err
	}
	if g.Retired {
		return Head{}, catalog.ErrDeleted
	}
	if err = s.currentContexts(ctx, tx, g); err != nil {
		return Head{}, err
	}
	if _, err = s.Catalog.ReviewDerivedTx(ctx, tx, g.Source, q.ExpectedActiveID); err != nil {
		return Head{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_heads(owner_id,view_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, actor.OwnerID, g.ViewKey); err != nil {
		return Head{}, err
	}
	head, err := readHead(ctx, tx, actor.OwnerID, g.ViewKey, true)
	if err != nil {
		return Head{}, err
	}
	if head.ActiveID != q.ExpectedActiveID || head.Revision != q.ExpectedRevision {
		return Head{}, catalog.ErrStale
	}
	var retired bool
	if err = tx.QueryRow(ctx, `SELECT retired FROM memory_consolidation_inputs WHERE source_id=$1 FOR SHARE`, g.InputID).Scan(&retired); err != nil {
		return Head{}, err
	}
	if retired {
		return Head{}, catalog.ErrDeleted
	}
	if head.ActiveID == g.Source.ID {
		return head, tx.Commit(ctx)
	}
	if head.ActiveID != "" {
		old, err := readGeneration(ctx, tx, actor.OwnerID, head.ActiveID)
		if err != nil {
			return Head{}, err
		}
		if _, err = s.Catalog.SetDerivedViewStateTx(ctx, tx, old.Source, "quarantined"); err != nil {
			return Head{}, err
		}
	}
	if _, err = s.Catalog.SetDerivedViewStateTx(ctx, tx, g.Source, "active"); err != nil {
		return Head{}, err
	}
	head.ActiveID = g.Source.ID
	head.Revision++
	if _, err = tx.Exec(ctx, `UPDATE memory_consolidation_heads SET active_source_id=$3,revision=$4 WHERE owner_id=$1 AND view_key=$2`, actor.OwnerID, g.ViewKey, head.ActiveID, head.Revision); err != nil {
		return Head{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_decisions(owner_id,view_key,revision,action,previous_source_id,selected_source_id) VALUES($1,$2,$3,'activate',$4,$5)`, actor.OwnerID, g.ViewKey, head.Revision, q.ExpectedActiveID, head.ActiveID); err != nil {
		return Head{}, err
	}
	return head, tx.Commit(ctx)
}

// Deactivate removes a current presentation even when its original evidence is
// now stale. It never needs to revive an old permission merely to turn it off.
func (s *Store) Deactivate(ctx context.Context, q Activation) (Head, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil {
		return Head{}, catalog.ErrDenied
	}
	if q.ExpectedActiveID == "" || q.ExpectedRevision < 1 || q.SourceID != "" {
		return Head{}, catalog.ErrInvalid
	}
	tx, err := s.Catalog.Pool().Begin(ctx)
	if err != nil {
		return Head{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return Head{}, err
	}
	g, err := readGeneration(ctx, tx, actor.OwnerID, q.ExpectedActiveID)
	if err != nil {
		return Head{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT source_id FROM memory_sources WHERE source_id=$1 FOR UPDATE`, g.Source.ID); err != nil {
		return Head{}, err
	}
	head, err := readHead(ctx, tx, actor.OwnerID, g.ViewKey, true)
	if err != nil {
		return Head{}, err
	}
	if head.ActiveID != q.ExpectedActiveID || head.Revision != q.ExpectedRevision {
		return Head{}, catalog.ErrStale
	}
	if _, err = s.Catalog.SetDerivedViewStateTx(ctx, tx, g.Source, "quarantined"); err != nil {
		return Head{}, err
	}
	head.ActiveID = ""
	head.Revision++
	if _, err = tx.Exec(ctx, `UPDATE memory_consolidation_heads SET active_source_id=NULL,revision=$3 WHERE owner_id=$1 AND view_key=$2`, actor.OwnerID, g.ViewKey, head.Revision); err != nil {
		return Head{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_decisions(owner_id,view_key,revision,action,previous_source_id) VALUES($1,$2,$3,'deactivate',$4)`, actor.OwnerID, g.ViewKey, head.Revision, q.ExpectedActiveID); err != nil {
		return Head{}, err
	}
	return head, tx.Commit(ctx)
}
