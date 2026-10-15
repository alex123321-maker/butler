package consolidation

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

type Retirement struct {
	InputID          string `json:"input_source_id"`
	ExpectedActiveID string `json:"expected_active_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type RetirementReceipt struct {
	InputID         string `json:"input_source_id"`
	Retired         bool   `json:"retired"`
	CleanupComplete bool   `json:"cleanup_complete"`
	Head            Head   `json:"head"`
}

// Retire first atomically fences selection, then uses the unchanged catalog
// privacy deletion/anchor to erase only this input and its result descendants.
// Failure leaves an irreversible retirement and explicit cleanup-pending receipt;
// such content still counts against retention limits until erasure completes.
func (s *Store) Retire(ctx context.Context, q Retirement) (RetirementReceipt, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil {
		return RetirementReceipt{}, catalog.ErrDenied
	}
	if q.InputID == "" || q.ExpectedRevision < 0 {
		return RetirementReceipt{}, catalog.ErrInvalid
	}
	tx, err := s.Catalog.Pool().Begin(ctx)
	if err != nil {
		return RetirementReceipt{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return RetirementReceipt{}, err
	}
	var policyID string
	if err = tx.QueryRow(ctx, `SELECT policy_id FROM memory_consolidation_inputs WHERE source_id=$1 AND owner_id=$2`, q.InputID, actor.OwnerID).Scan(&policyID); err != nil {
		return RetirementReceipt{}, catalog.ErrDenied
	}
	in, err := readInput(ctx, tx, policyID)
	if err != nil {
		return RetirementReceipt{}, err
	}
	var sourceRef catalog.SourceRef
	if err = tx.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,v.content_hash FROM memory_sources s JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision WHERE s.source_id=$1 AND s.owner_id=$2`, in.SourceID, actor.OwnerID).Scan(&sourceRef.ID, &sourceRef.Revision, &sourceRef.AccessRevision, &sourceRef.Hash); err != nil {
		return RetirementReceipt{}, err
	}
	ids := []string{in.SourceID}
	rows, err := tx.Query(ctx, `SELECT source_id FROM memory_consolidation_generations WHERE input_source_id=$1 AND owner_id=$2`, in.SourceID, actor.OwnerID)
	if err != nil {
		return RetirementReceipt{}, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return RetirementReceipt{}, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return RetirementReceipt{}, err
	}
	sort.Strings(ids)
	if _, err = tx.Exec(ctx, `SELECT source_id FROM memory_sources WHERE source_id=ANY($1) ORDER BY source_id FOR UPDATE`, ids); err != nil {
		return RetirementReceipt{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_heads(owner_id,view_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, actor.OwnerID, in.ViewKey); err != nil {
		return RetirementReceipt{}, err
	}
	head, err := readHead(ctx, tx, actor.OwnerID, in.ViewKey, true)
	if err != nil {
		return RetirementReceipt{}, err
	}
	if !in.Retired {
		if head.ActiveID != q.ExpectedActiveID || head.Revision != q.ExpectedRevision {
			return RetirementReceipt{}, catalog.ErrStale
		}
		for _, id := range ids {
			if id == head.ActiveID {
				return RetirementReceipt{}, catalog.ErrDenied
			}
		}
		for _, id := range ids {
			var ref catalog.SourceRef
			if id == in.SourceID {
				ref = sourceRef
			} else {
				g, err := readGeneration(ctx, tx, actor.OwnerID, id)
				if err != nil {
					return RetirementReceipt{}, err
				}
				ref = g.Source
			}
			if _, err = s.Catalog.SetDerivedViewStateTx(ctx, tx, ref, "quarantined"); err != nil {
				return RetirementReceipt{}, err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE memory_consolidation_inputs SET retired=TRUE WHERE source_id=$1 AND owner_id=$2`, in.SourceID, actor.OwnerID); err != nil {
			return RetirementReceipt{}, err
		}
		// Retirement is a versioned owner decision even if the active pointer is
		// unchanged. A stale review from before it cannot silently overwrite it.
		head.Revision++
		if _, err = tx.Exec(ctx, `UPDATE memory_consolidation_heads SET revision=$3 WHERE owner_id=$1 AND view_key=$2`, actor.OwnerID, in.ViewKey, head.Revision); err != nil {
			return RetirementReceipt{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_decisions(owner_id,view_key,revision,action,previous_source_id,selected_source_id) VALUES($1,$2,$3,'retire',$4,$4)`, actor.OwnerID, in.ViewKey, head.Revision, head.ActiveID); err != nil {
			return RetirementReceipt{}, err
		}
	}
	receipt := RetirementReceipt{InputID: in.SourceID, Retired: true, CleanupComplete: in.CleanupComplete, Head: head}
	if err = tx.Commit(ctx); err != nil {
		return RetirementReceipt{}, err
	}
	if receipt.CleanupComplete {
		return receipt, nil
	}
	// Do not hold a source/head transaction while Delete acquires the existing
	// sessions→tasks→heads→sources privacy order or fsyncs the independent anchor.
	inspected, err := s.Catalog.Inspect(ctx, in.SourceID)
	if err != nil {
		return receipt, nil
	}
	if inspected.Source.Status != "deleted" && inspected.Source.Status != "revoked" {
		if _, err = s.Catalog.Delete(ctx, in.SourceID, "delete", sourceRef.Revision); err != nil {
			return receipt, nil
		}
	}
	if _, err = s.Catalog.Pool().Exec(ctx, `UPDATE memory_consolidation_inputs SET cleanup_complete=TRUE WHERE source_id=$1 AND owner_id=$2 AND retired`, in.SourceID, actor.OwnerID); err != nil {
		return receipt, nil
	}
	receipt.CleanupComplete = true
	return receipt, nil
}

// List returns content-free retained preparations plus bounded completed
// history. Pending cleanup cannot disappear behind a history pagination cap.
func (s *Store) List(ctx context.Context) ([]map[string]any, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil {
		return nil, catalog.ErrDenied
	}
	if err := s.Catalog.CheckRestore(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Catalog.Pool().Query(ctx, `SELECT to_jsonb(i)||jsonb_build_object('generation_source_id',g.source_id,'active_source_id',h.active_source_id,'head_revision',COALESCE(h.revision,0)) FROM memory_consolidation_inputs i LEFT JOIN memory_consolidation_generations g ON g.input_source_id=i.source_id LEFT JOIN memory_consolidation_heads h ON h.owner_id=i.owner_id AND h.view_key=i.view_key WHERE i.owner_id=$1 AND (NOT i.cleanup_complete OR i.source_id IN(SELECT source_id FROM memory_consolidation_inputs WHERE owner_id=$1 AND cleanup_complete ORDER BY created_at DESC LIMIT 64)) ORDER BY i.created_at DESC,i.source_id`, actor.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item map[string]any
		if json.Unmarshal(raw, &item) != nil {
			return nil, catalog.ErrInvalid
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
