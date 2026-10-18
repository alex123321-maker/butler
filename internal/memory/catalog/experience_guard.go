package catalog

import (
	"context"
	"github.com/butler/butler/internal/ownership"

	"github.com/jackc/pgx/v5"
)

// ExperienceMetadataEligibleTx is a content-free prefilter only. It never grants
// source access. Full current Catalog proof is mandatory before release/pinning.
func ExperienceMetadataEligibleTx(ctx context.Context, tx pgx.Tx, ids []string, environment string) (bool, error) {
	if len(ids) > 64 {
		return false, ErrDenied
	}
	var eligible bool
	err := tx.QueryRow(ctx, `SELECT butler_memory_experience_lessons_eligible($1,$2)`, ids, environment).Scan(&eligible)
	return eligible, err
}

// BindExperienceLineageTx repeats full Catalog proof, then binds every lesson
// ancestor. The caller obtains current Broker contracts outside SQL and keeps
// the existing reviewed-skills installed-dependency validator. Empty run is a
// proof-only check. Historical owner inspection need not call this runtime gate.
func (s *Store) BindExperienceLineageTx(ctx context.Context, tx pgx.Tx, roots []SourceRef, environment, run string) error {
	ids := []string{}
	for _, r := range roots {
		ids = append(ids, r.ID)
	}
	eligible, err := ExperienceMetadataEligibleTx(ctx, tx, ids, environment)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrDenied
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE roots AS(SELECT unnest($1::text[]) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id) SELECT l.source_id,l.lesson_hash FROM memory_experience_lessons l JOIN roots r ON r.id=l.source_id ORDER BY l.source_id LIMIT 9`, ids)
	if err != nil {
		return err
	}
	type lesson struct{ id, hash string }
	lessons := []lesson{}
	for rows.Next() {
		var l lesson
		if err = rows.Scan(&l.id, &l.hash); err != nil {
			rows.Close()
			return err
		}
		lessons = append(lessons, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(lessons) > 8 {
		return ErrDenied
	}
	if len(lessons) == 0 {
		return nil
	}
	if _, err = s.AccessTx(ctx, tx, roots, "memory_read"); err != nil {
		return err
	}
	// Source rows precede lesson/claim metadata, matching feedback and ordinary
	// source correction. No metadata lock is held while acquiring source locks.
	lessonIDs := []string{}
	for _, l := range lessons {
		lessonIDs = append(lessonIDs, l.id)
	}
	if _, err = tx.Exec(ctx, `SELECT l.source_id FROM memory_experience_lessons l JOIN memory_claims c ON c.claim_id=l.claim_id WHERE l.source_id=ANY($1) ORDER BY l.source_id FOR SHARE OF l,c`, lessonIDs); err != nil {
		return err
	}
	if eligible, err = ExperienceMetadataEligibleTx(ctx, tx, ids, environment); err != nil {
		return err
	} else if !eligible {
		return ErrDenied
	}
	if run == "" {
		return nil
	}
	if token, ok := ownership.TokenFromContext(ctx); !ok || token.RunID != run {
		return ErrDenied
	}
	// Eight unique ordinal slots enforce the union bound even for concurrent
	// reads. Retrying a slot conflict is bounded and acquires no new lock class.
	for _, l := range lessons {
		bound := false
		for attempt := 0; attempt < 8; attempt++ {
			_, err = tx.Exec(ctx, `INSERT INTO memory_experience_pins(run_id,source_id,environment_hash,lesson_hash,slot)
 SELECT $1,$2,$3,$4,n FROM generate_series(1,8)n WHERE NOT EXISTS(SELECT 1 FROM memory_experience_pins WHERE run_id=$1 AND slot=n) ORDER BY n LIMIT 1 ON CONFLICT DO NOTHING`, run, l.id, environment, l.hash)
			if err != nil {
				return err
			}
			var env, digest string
			err = tx.QueryRow(ctx, `SELECT environment_hash,lesson_hash FROM memory_experience_pins WHERE run_id=$1 AND source_id=$2`, run, l.id).Scan(&env, &digest)
			if err == nil {
				if env != environment || digest != l.hash {
					return ErrStale
				}
				bound = true
				break
			}
			if err != pgx.ErrNoRows {
				return err
			}
		}
		if !bound {
			return ErrDenied
		}
	}
	return nil
}
