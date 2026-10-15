package consolidation

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/jackc/pgx/v5"
)

func (Handler) StagedModelResult() bool { return true }

func currentInput(ctx context.Context, tx pgx.Tx, c *catalog.Store, j jobs.Job, sources []catalog.Source) (inputRecord, catalog.Source, Envelope, error) {
	in, err := readInput(ctx, tx, j.PolicyID)
	if err != nil || in.OwnerID != j.OwnerID || in.Retired {
		return in, catalog.Source{}, Envelope{}, jobs.ErrClaim
	}
	var anchor catalog.Source
	for _, s := range sources {
		if s.ID == in.SourceID {
			anchor = s
		}
	}
	if anchor.ID == "" {
		return in, anchor, Envelope{}, jobs.ErrClaim
	}
	page, err := c.ReadAcceptedFactsTx(ctx, tx, in.ClaimIDs, in.Query)
	if err != nil {
		return in, anchor, Envelope{}, err
	}
	if len(page.Items) != len(in.ClaimIDs) || page.Truncated || page.UnknownValidity > 0 || page.NotKnownAtObservation > 0 {
		return in, anchor, Envelope{}, catalog.ErrStale
	}
	envelope, err := buildEnvelope(sources, in.Sources, page.Items)
	if err != nil {
		return in, anchor, envelope, err
	}
	if string(mustJSON(envelope)) != anchor.Body {
		return in, anchor, envelope, catalog.ErrStale
	}
	return in, anchor, envelope, nil
}

// InputSourcesTx shows only the exact trusted envelope; complete originals stay
// in the immutable job manifest and every independent network/publication proof.
func (Handler) InputSourcesTx(ctx context.Context, tx pgx.Tx, c *catalog.Store, j jobs.Job, sources []catalog.Source) ([]catalog.Source, error) {
	_, anchor, _, err := currentInput(ctx, tx, c, j, sources)
	if err != nil {
		return nil, err
	}
	return []catalog.Source{anchor}, nil
}

// PrepareSourcesTx keeps one-use snapshots exact. Stale parents/decisions require
// fresh owner preparation, never a hidden rebase to new facts or permission.
func (Handler) PrepareSourcesTx(ctx context.Context, tx pgx.Tx, c *catalog.Store, p jobs.ModelPolicy, sources []catalog.Source) ([]catalog.Source, string, error) {
	if p.MaxJobs != 1 || p.Handler != Version {
		return nil, "", jobs.ErrClaim
	}
	in, err := readInput(ctx, tx, p.ID)
	if err != nil || in.OwnerID != p.OwnerID || in.Retired {
		return nil, "", jobs.ErrClaim
	}
	found := false
	for _, src := range sources {
		if src.ID == in.SourceID {
			found = true
			if _, err = readEnvelope([]catalog.Source{src}); err != nil {
				return nil, "", err
			}
		}
	}
	if !found {
		return nil, "", jobs.ErrClaim
	}
	// AccessTx already checked every current parent and the anchor's bound
	// temporal pins. Fresh typed values are reconstructed again in admission;
	// the scheduler does not acquire an additional read purpose from its policy.
	return sources, "", nil
}

func selectionOf(result Representation) Selection {
	out := Selection{Groups: []SelectedGroup{}}
	for _, g := range result.Groups {
		members := []string{}
		for _, v := range g.Variants {
			for _, e := range v.Evidence {
				members = append(members, e.ID)
			}
		}
		sort.Strings(members)
		out.Groups = append(out.Groups, SelectedGroup{Members: members, Relation: g.ModelRelation})
	}
	return out
}

func unitIDs(result Representation) []string {
	ids := []string{}
	for _, g := range result.Groups {
		for _, v := range g.Variants {
			for _, e := range v.Evidence {
				ids = append(ids, e.ID)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func compareIDs(previous []string, next Representation) Diff {
	old := Representation{Groups: []Group{{Variants: []Variant{{Evidence: []Evidence{}}}}}}
	for _, id := range previous {
		old.Groups[0].Variants[0].Evidence = append(old.Groups[0].Variants[0].Evidence, Evidence{ID: id})
	}
	return Compare(&old, next)
}

// PublishTx atomically binds the staged immutable result to exact input/temporal
// parents and the existing observed attempt. It changes no owner fact head.
func (h Handler) PublishTx(ctx context.Context, tx pgx.Tx, c *catalog.Store, j jobs.Job, a jobs.ModelAttempt, sources []catalog.Source, src catalog.Source, body string) error {
	if j.Purpose != Version || src.Status != "quarantined" {
		return jobs.ErrClaim
	}
	in, anchor, envelope, err := currentInput(ctx, tx, c, j, sources)
	if err != nil {
		return err
	}
	var result Representation
	if strict(body, &result) != nil {
		return jobs.ErrClaim
	}
	checked, err := h.Validate([]catalog.Source{anchor}, string(mustJSON(selectionOf(result))))
	if err != nil || checked != body {
		return jobs.ErrClaim
	}
	if pins := pinsOf(envelope); len(pins) > 0 {
		if err = c.BindNewAcceptedFactPinsTx(ctx, tx, src, j.Sources, pins, in.Query); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_heads(owner_id,view_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, j.OwnerID, in.ViewKey); err != nil {
		return err
	}
	var base string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(active_source_id,''),revision FROM memory_consolidation_heads WHERE owner_id=$1 AND view_key=$2 FOR UPDATE`, j.OwnerID, in.ViewKey).Scan(&base, &revision); err != nil {
		return err
	}
	previous := []string{}
	if base != "" {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT unit_ids FROM memory_consolidation_generations WHERE source_id=$1 AND owner_id=$2`, base, j.OwnerID).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &previous) != nil {
			return catalog.ErrInvalid
		}
	}
	var retired bool
	if err = tx.QueryRow(ctx, `SELECT retired FROM memory_consolidation_inputs WHERE source_id=$1`, in.SourceID).Scan(&retired); err != nil {
		return err
	}
	if retired {
		return catalog.ErrStale
	}
	_, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_generations(source_id,input_source_id,owner_id,view_key,algorithm,operation_id,source_revision,content_hash,parent_manifest,unit_ids,base_source_id,base_revision,diff) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, src.ID, in.SourceID, j.OwnerID, in.ViewKey, Version, a.OperationID, src.Revision, src.Hash, mustJSON(j.Sources), mustJSON(unitIDs(result)), base, revision, mustJSON(compareIDs(previous, result)))
	return err
}
