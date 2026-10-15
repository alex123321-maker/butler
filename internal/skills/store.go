package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool        *pgxpool.Pool
	owner       string
	verifier    EvidenceVerifier
	contracts   ContractReader
	environment RunEnvironment
	sources     *catalog.Store
}

type proposalAuthorityKey struct{}
type proposalSourceKey struct{}

func (s *Store) guardProposalTx(ctx context.Context, tx pgx.Tx) error {
	if c, ok := ctx.Value(proposalAuthorityKey{}).(actionauth.ContextSnapshot); ok {
		return s.checkRunTx(ctx, tx, c)
	}
	return nil
}

func NewStore(pool *pgxpool.Pool, owner string, verifier EvidenceVerifier, contracts ContractReader, environment RunEnvironment) *Store {
	return &Store{pool: pool, owner: owner, verifier: verifier, contracts: contracts, environment: environment}
}

// SetSources is startup-only wiring to the owned deletion-aware catalog.
func (s *Store) SetSources(sources *catalog.Store) { s.sources = sources }

// SetContracts is immutable bootstrap composition for current reviewed catalogs.
func (s *Store) SetContracts(contracts ContractReader) { s.contracts = contracts }
func (s *Store) actor(ctx context.Context) (owneridentity.Actor, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.pool == nil || s.owner == "" || a.OwnerID != s.owner || a.DeliveryID == "" {
		return a, ErrAccess
	}
	return a, nil
}
func encoded(v any) string { b, _ := json.Marshal(v); return string(b) }

type query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) metadata(ctx context.Context, q query, hash string) (Record, error) {
	var r Record
	var ref, deps []byte
	err := q.QueryRow(ctx, `SELECT v.skill_id,v.owner_id,v.digest,c.generation,s.status,COALESCE(c.active_digest=v.digest,false),v.version,v.source_ref_json,v.dependencies_json,v.correction_shape FROM skill_versions v JOIN skill_states s USING(digest) JOIN skill_catalog c ON c.skill_id=v.skill_id WHERE v.digest=$1 AND v.owner_id=$2`, hash, s.owner).Scan(&r.ID, &r.OwnerID, &r.Digest, &r.Generation, &r.Status, &r.Active, &r.Version, &ref, &deps, &r.shape)
	if err != nil {
		return r, ErrUnavailable
	}
	if json.Unmarshal(ref, &r.BodySource) != nil || json.Unmarshal(deps, &r.dependencies) != nil || r.BodySource.ID == "" || len(r.dependencies) == 0 {
		return r, ErrUnavailable
	}
	return r, nil
}
func (s *Store) read(ctx context.Context, tx pgx.Tx, hash string) (Record, error) {
	r, err := s.metadata(ctx, tx, hash)
	if err != nil {
		return r, err
	}
	if s.sources == nil {
		return r, ErrUnavailable
	}
	data, err := s.sources.AccessTx(ctx, tx, []catalog.SourceRef{r.BodySource}, "memory_read")
	if err != nil || len(data) != 1 {
		return r, ErrUnavailable
	}
	p, err := Parse([]byte(data[0].Body))
	if err != nil || p.Digest != hash || p.Manifest.ID != r.ID || p.Manifest.Version != r.Version || encoded(p.Manifest.Dependencies) != encoded(r.dependencies) || CorrectionShape(p.Manifest) != r.shape {
		return r, ErrUnavailable
	}
	r.Package = p
	r.BodyAvailable = true
	return r, nil
}
func (s *Store) Get(ctx context.Context, hash string) (Record, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, hash)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func event(ctx context.Context, tx pgx.Tx, r Record, kind string, source any) error {
	_, err := tx.Exec(ctx, `INSERT INTO skill_events(skill_id,digest,generation,kind,source_json) VALUES($1,$2,$3,$4,$5::jsonb)`, r.ID, r.Digest, r.Generation, kind, encoded(source))
	return err
}
func (s *Store) Stage(ctx context.Context, p Package) (Record, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	return s.stage(ctx, p, a)
}
func (s *Store) stage(ctx context.Context, p Package, source any) (Record, error) {
	fresh, err := Parse(p.Raw)
	if err != nil || fresh.Digest != p.Digest {
		return Record{}, ErrManifest
	}
	if s.sources == nil {
		return Record{}, ErrUnavailable
	}
	// Provenance is independently resolved before opening the publication
	// transaction. Submitted task IDs or caller test reports grant no access.
	report, err := Validate(ctx, p, s.owner, s.verifier, s.contracts)
	if err != nil {
		return Record{}, err
	}
	parents := []catalog.SourceRef{}
	seen := map[string]catalog.SourceRef{}
	for _, proof := range report.Sources {
		for _, ref := range proof.ContentParents {
			if old, ok := seen[ref.ID]; ok {
				if old != ref {
					return Record{}, ErrEvidence
				}
				continue
			}
			seen[ref.ID] = ref
			parents = append(parents, ref)
		}
	}
	for _, r := range report.LessonParents {
		if old, ok := seen[r.ID]; ok && old != r {
			return Record{}, ErrEvidence
		}
		if _, ok := seen[r.ID]; !ok {
			seen[r.ID] = r
			parents = append(parents, r)
		}
	}
	if ref, ok := ctx.Value(proposalSourceKey{}).(catalog.SourceRef); ok {
		if old, exists := seen[ref.ID]; exists && old != ref {
			return Record{}, ErrEvidence
		}
		if _, exists := seen[ref.ID]; !exists {
			parents = append(parents, ref)
		}
	}
	if len(parents) == 0 || len(parents) > 64 {
		return Record{}, ErrEvidence
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardProposalTx(ctx, tx); err != nil {
		return Record{}, err
	}
	// Source graph locks precede skill lifecycle locks. Text exists only in the
	// deletion-aware catalog; immutable skill history contains no procedure prose.
	var bodyRef catalog.SourceRef
	var existing []byte
	if e := tx.QueryRow(ctx, `SELECT source_ref_json FROM skill_versions WHERE digest=$1 AND owner_id=$2`, p.Digest, s.owner).Scan(&existing); e == nil {
		if json.Unmarshal(existing, &bodyRef) != nil {
			return Record{}, ErrUnavailable
		}
		if _, e = s.sources.AccessTx(ctx, tx, []catalog.SourceRef{bodyRef}, "memory_read"); e != nil {
			return Record{}, ErrUnavailable
		}
	} else if e == pgx.ErrNoRows {
		body, e := s.sources.CreateDerivedTx(ctx, tx, parents, "memory_read", p.Manifest.ID, string(p.Raw), json.RawMessage(encoded(map[string]string{"kind": "reviewed_skill_v1", "digest": p.Digest})))
		if e != nil {
			return Record{}, e
		}
		bodyRef = catalog.SourceRef{ID: body.ID, Revision: body.Revision, AccessRevision: body.AccessRevision, Hash: body.Hash}
	} else {
		return Record{}, e
	}
	if _, err = tx.Exec(ctx, `INSERT INTO skill_catalog(skill_id,owner_id,generation) VALUES($1,$2,1) ON CONFLICT DO NOTHING`, p.Manifest.ID, s.owner); err != nil {
		return Record{}, err
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM skill_catalog WHERE skill_id=$1 FOR UPDATE`, p.Manifest.ID).Scan(&owner); err != nil || owner != s.owner {
		return Record{}, ErrAccess
	}
	if _, err = tx.Exec(ctx, `INSERT INTO skill_versions(digest,skill_id,version,owner_id,source_ref_json,dependencies_json,correction_shape) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7) ON CONFLICT DO NOTHING`, p.Digest, p.Manifest.ID, p.Manifest.Version, s.owner, encoded(bodyRef), encoded(p.Manifest.Dependencies), CorrectionShape(p.Manifest)); err != nil {
		return Record{}, err
	}
	// A conflicting semantic version cannot silently point at new raw bytes.
	var hash string
	if err = tx.QueryRow(ctx, `SELECT digest FROM skill_versions WHERE owner_id=$1 AND skill_id=$2 AND version=$3`, s.owner, p.Manifest.ID, p.Manifest.Version).Scan(&hash); err != nil || hash != p.Digest {
		return Record{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO skill_states(digest,status) VALUES($1,'candidate') ON CONFLICT DO NOTHING`, p.Digest); err != nil {
		return Record{}, err
	}
	r, err := s.metadata(ctx, tx, p.Digest)
	if err != nil {
		return r, err
	}
	r.Package = p
	r.BodyAvailable = true
	if err = event(ctx, tx, r, "candidate", source); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (s *Store) Test(ctx context.Context, hash string) (Validation, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return Validation{}, err
	}
	return s.test(ctx, hash, a)
}
func (s *Store) test(ctx context.Context, hash string, source any) (Validation, error) {
	r, err := s.Get(ctx, hash)
	if err != nil || r.Status == "revoked" {
		return Validation{}, ErrUnavailable
	}
	report, validationErr := Validate(ctx, r.Package, s.owner, s.verifier, s.contracts)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardProposalTx(ctx, tx); err != nil {
		return report, err
	}
	if _, err = s.read(ctx, tx, hash); err != nil {
		return report, err
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM skill_states WHERE digest=$1 FOR UPDATE`, hash).Scan(&status); err != nil || status == "revoked" {
		return report, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO skill_validations(digest,report_json) VALUES($1,$2::jsonb)`, hash, encoded(report)); err != nil {
		return report, err
	}
	if validationErr == nil && status == "candidate" {
		if _, err = tx.Exec(ctx, `UPDATE skill_states SET status='tested' WHERE digest=$1`, hash); err != nil {
			return report, err
		}
	}
	if err = event(ctx, tx, r, "tested", source); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	return report, validationErr
}
func (s *Store) Activate(ctx context.Context, hash string, generation int64) (Record, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	r, err := s.Get(ctx, hash)
	if err != nil {
		return r, err
	}
	if _, err = Validate(ctx, r.Package, s.owner, s.verifier, s.contracts); err != nil {
		return r, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	r, err = s.activate(ctx, tx, hash, generation, a)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (s *Store) activate(ctx context.Context, tx pgx.Tx, hash string, generation int64, source any) (Record, error) {
	r, err := s.read(ctx, tx, hash)
	if err != nil {
		return r, err
	}
	var current int64
	var old string
	if err = tx.QueryRow(ctx, `SELECT generation,COALESCE(active_digest,'') FROM skill_catalog WHERE skill_id=$1 AND owner_id=$2 FOR UPDATE`, r.ID, s.owner).Scan(&current, &old); err != nil || current != generation {
		return r, ErrConflict
	}
	var status string
	var passed bool
	if err = tx.QueryRow(ctx, `SELECT status,COALESCE((SELECT (report_json->>'passed')::boolean FROM skill_validations WHERE digest=$1 ORDER BY validation_id DESC LIMIT 1),false) FROM skill_states WHERE digest=$1 FOR UPDATE`, hash).Scan(&status, &passed); err != nil || !passed || status == "candidate" || status == "revoked" {
		return r, ErrEvidence
	}
	deps := map[string]Dependency{}
	for _, d := range r.dependencies {
		deps[d.Tool] = d
	}
	// A previous HTTP validation does not authorize activation after a
	// concurrent interface change. Check current owned contracts in this TX.
	if err = s.checkDependenciesTx(ctx, tx, deps); err != nil {
		return r, err
	}
	if old != "" && old != hash {
		if _, err = tx.Exec(ctx, `UPDATE skill_states SET status='obsolete' WHERE digest=$1 AND status='active'`, old); err != nil {
			return r, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE skill_catalog SET active_digest=$2,generation=generation+1 WHERE skill_id=$1`, r.ID, hash); err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `UPDATE skill_states SET status='active' WHERE digest=$1`, hash); err != nil {
		return r, err
	}
	r.Generation = current + 1
	r.Active = true
	r.Status = "active"
	if err = event(ctx, tx, r, "activated", source); err != nil {
		return r, err
	}
	return r, nil
}

// Revoke is permanent for this digest. Disable merely retires an active version;
// rollback may explicitly reactivate a tested obsolete version.
func (s *Store) Disable(ctx context.Context, hash string, generation int64, revoke bool) (Record, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	r, err := s.metadata(ctx, tx, hash)
	if err != nil || r.Status == "revoked" {
		if err == nil {
			err = ErrUnavailable
		}
		return r, err
	}
	var current int64
	if err = tx.QueryRow(ctx, `SELECT generation FROM skill_catalog WHERE skill_id=$1 FOR UPDATE`, r.ID).Scan(&current); err != nil || current != generation {
		return r, ErrConflict
	}
	status := "obsolete"
	if revoke {
		status = "revoked"
	}
	if _, err = tx.Exec(ctx, `UPDATE skill_states SET status=$2 WHERE digest=$1 AND status<>'revoked'`, hash, status); err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `UPDATE skill_catalog SET active_digest=CASE WHEN active_digest=$2 THEN NULL ELSE active_digest END,generation=generation+1 WHERE skill_id=$1`, r.ID, hash); err != nil {
		return r, err
	}
	r.Generation = current + 1
	r.Status = status
	r.Active = false
	if err = event(ctx, tx, r, status, a); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if _, err := s.actor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT digest FROM skill_versions WHERE owner_id=$1 ORDER BY skill_id,created_at,digest`, s.owner)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []Record{}
	for _, id := range ids {
		r, e := s.Get(ctx, id)
		if e != nil {
			// Content removal keeps digest/lifecycle audit inspectable, while no
			// body can be returned from a deleted or restored stale source.
			r, e = s.metadata(ctx, s.pool, id)
			if e != nil {
				return nil, e
			}
		}
		result = append(result, r)
	}
	return result, nil
}
func (s *Store) GrantCorrection(ctx context.Context, g CorrectionGrant) error {
	a, err := s.actor(ctx)
	if err != nil {
		return err
	}
	if !identifier.MatchString(g.ID) || !identifier.MatchString(g.SkillID) || !digest.MatchString(g.ParentDigest) || g.Class != "descriptive_correction_v1" || g.MandateID == "" || g.MandateRevision <= 0 || strings.TrimSpace(g.Purpose) == "" || !g.ExpiresAt.After(time.Now()) || g.ExpiresAt.After(time.Now().Add(7*24*time.Hour)) {
		return ErrManifest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := s.metadata(ctx, tx, g.ParentDigest)
	if err != nil || !r.Active || r.ID != g.SkillID {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO skill_correction_grants(grant_id,skill_id,parent_digest,owner_id,class,mandate_id,mandate_revision,purpose,expires_at,source_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)`, g.ID, g.SkillID, g.ParentDigest, s.owner, g.Class, g.MandateID, g.MandateRevision, g.Purpose, g.ExpiresAt, encoded(a))
	if err != nil {
		return err
	}
	if err = event(ctx, tx, r, "correction_grant_created", a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) RevokeCorrection(ctx context.Context, id string) error {
	a, err := s.actor(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hash string
	if err = tx.QueryRow(ctx, `UPDATE skill_correction_grants SET revoked=true WHERE grant_id=$1 AND owner_id=$2 RETURNING parent_digest`, id, s.owner).Scan(&hash); err != nil {
		return ErrUnavailable
	}
	r, err := s.metadata(ctx, tx, hash)
	if err != nil {
		return err
	}
	if err = event(ctx, tx, r, "correction_grant_revoked", a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) String() string {
	return fmt.Sprintf("reviewed skills for configured owner %t", s.owner != "")
}
