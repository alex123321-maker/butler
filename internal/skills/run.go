package skills

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *Store) run(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	if s == nil || s.environment == nil || s.owner == "" {
		return actionauth.ContextSnapshot{}, ErrUnavailable
	}
	c, err := s.environment.GetActionContext(ctx, id)
	if err != nil || c.RunID != id || c.OwnerID != s.owner || c.LeaseID == "" || c.LeaseEpoch <= 0 || !c.LeaseExpiresAt.After(time.Now()) || c.TaskID == "" || c.TaskStatus != "active" || c.TaskRevision != c.PinnedTaskRevision || c.ConstraintsRevision <= 0 {
		return c, ErrAccess
	}
	switch c.RunState {
	case "preparing", "model_running", "tool_pending", "tool_running", "awaiting_approval", "awaiting_model_resume", "RUN_STATE_PREPARING", "RUN_STATE_MODEL_RUNNING", "RUN_STATE_TOOL_PENDING", "RUN_STATE_TOOL_RUNNING", "RUN_STATE_AWAITING_APPROVAL", "RUN_STATE_AWAITING_MODEL_RESUME":
		return c, nil
	}
	return c, ErrAccess
}
func (s *Store) checkRunTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot) error {
	var lease, run string
	var epoch int64
	var expires time.Time
	if err := tx.QueryRow(ctx, `SELECT lease_id,run_id,fence_epoch,expires_at FROM session_ownership WHERE session_key=$1 FOR SHARE`, c.SessionKey).Scan(&lease, &run, &epoch, &expires); err != nil || lease != c.LeaseID || run != c.RunID || epoch != c.LeaseEpoch {
		return ErrAccess
	}
	var at time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil || !expires.After(at) {
		return ErrAccess
	}
	var sessionOwner string
	if err := tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR KEY SHARE`, c.SessionKey).Scan(&sessionOwner); err != nil || sessionOwner != c.OwnerID {
		return ErrAccess
	}
	var pinned string
	var pinnedRev int64
	// Exclusive run lock also serializes bounded usage insertion. Acquire it
	// before the task lock; never upgrade parallel shared run locks later.
	if err := tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0) FROM runs WHERE run_id=$1 FOR UPDATE`, c.RunID).Scan(&pinned, &pinnedRev); err != nil || pinned != c.TaskID || pinnedRev != c.TaskRevision {
		return ErrAccess
	}
	var revision, constraints int64
	var owner, status string
	if err := tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, c.TaskID).Scan(&owner, &status, &revision, &constraints); err != nil || owner != c.OwnerID || status != "active" || revision != c.TaskRevision || constraints != c.ConstraintsRevision {
		return ErrAccess
	}
	_, err := actionauth.CheckTaskAuthorityTx(ctx, tx, c.TaskID, c.OwnerID, c.Authority, at)
	return err
}
func runContext(ctx context.Context, c actionauth.ContextSnapshot) context.Context {
	// Even an owner control call routed to a run must use current run authority,
	// not accidentally inherit the actor's unrestricted owner memory view.
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{})
	return ownership.WithToken(ctx, nil, ownership.Token{SessionKey: c.SessionKey, RunID: c.RunID, LeaseID: c.LeaseID, FenceEpoch: c.LeaseEpoch})
}
func dependenciesMatch(p Package, contracts []*toolbrokerv1.ToolContract) bool {
	byName := map[string]*toolbrokerv1.ToolContract{}
	for _, c := range contracts {
		byName[c.GetToolName()] = c
	}
	for _, d := range p.Manifest.Dependencies {
		c := byName[d.Tool]
		hash, err := actionauth.ContractHash(c)
		if err != nil || hash != d.ContractHash || c.GetContractRevision() != d.ContractRevision {
			return false
		}
	}
	return true
}
func (s *Store) Index(ctx context.Context, run, query string, limit int) ([]Descriptor, error) {
	c, err := s.run(ctx, run)
	if err != nil || len(query) > 256 {
		return nil, ErrAccess
	}
	ctx = runContext(ctx, c)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	if e = s.checkRunTx(ctx, tx, c); e != nil {
		_ = tx.Rollback(ctx)
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	contracts, err := s.environment.ContractsForRun(ctx, run)
	if err != nil {
		return nil, ErrUnavailable
	}
	rows, err := s.pool.Query(ctx, `SELECT active_digest FROM skill_catalog WHERE owner_id=$1 AND active_digest IS NOT NULL ORDER BY skill_id`, c.OwnerID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, hash)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []Descriptor{}
	terms := strings.Fields(strings.ToLower(query))
	for _, hash := range ids {
		r, e := s.Get(ctx, hash)
		if e != nil || !r.Active || r.Status != "active" || !dependenciesMatch(r.Package, contracts) {
			continue
		}
		matches := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(r.ID+" "+r.Package.Manifest.Description), term) {
				matches = false
			}
		}
		if matches {
			result = append(result, Descriptor{r.ID, r.Digest, r.Package.Manifest.Version, r.Package.Manifest.Description})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if limit <= 0 || limit > 12 {
		limit = 12
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (s *Store) Load(ctx context.Context, run, hash string, maxBytes int) (Package, error) {
	c, err := s.run(ctx, run)
	if err != nil {
		return Package{}, err
	}
	ctx = runContext(ctx, c)
	contracts, err := s.environment.ContractsForRun(ctx, run)
	if err != nil {
		return Package{}, ErrConflict
	}
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	if maxBytes > 16384 {
		return Package{}, ErrManifest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Package{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.checkRunTx(ctx, tx, c); err != nil {
		return Package{}, err
	}
	r, err := s.read(ctx, tx, hash)
	if err != nil {
		return Package{}, err
	}
	if !dependenciesMatch(r.Package, contracts) {
		return Package{}, ErrConflict
	}
	data, _ := json.Marshal(r.Package.Manifest)
	if len(data) > maxBytes {
		return Package{}, ErrManifest
	}
	var status string
	var active, pinned bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(active_digest=$2,false) FROM skill_catalog WHERE skill_id=$1 FOR SHARE`, r.ID, hash).Scan(&active); err != nil {
		return Package{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT status,EXISTS(SELECT 1 FROM skill_usages WHERE run_id=$2 AND digest=$1) FROM skill_states WHERE digest=$1 FOR SHARE`, hash, run).Scan(&status, &pinned); err != nil || status == "revoked" || !active && !(pinned && status == "obsolete") {
		return Package{}, ErrUnavailable
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM skill_usages WHERE run_id=$1`, run).Scan(&count); err != nil || !pinned && count >= 4 {
		return Package{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO skill_usages(run_id,digest,task_id,task_revision,constraints_revision) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, run, hash, c.TaskID, c.TaskRevision, c.ConstraintsRevision); err != nil {
		return Package{}, err
	}
	if err = s.CheckRunTx(ctx, tx, run, c.OwnerID); err != nil {
		return Package{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Package{}, err
	}
	return r.Package, nil
}
func (s *Store) Pinned(ctx context.Context, run string) ([]Package, error) {
	// A conversation without loaded procedures has no procedural authority to
	// check. Runs with pins are checked by Load under current owned task locks.
	rows, err := s.pool.Query(ctx, `SELECT digest FROM skill_usages WHERE run_id=$1 ORDER BY loaded_at,digest`, run)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, hash)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []Package{}
	for _, hash := range ids {
		p, e := s.Load(ctx, run, hash, 16384)
		if e != nil {
			return nil, e
		}
		result = append(result, p)
	}
	return result, nil
}
func (s *Store) Propose(ctx context.Context, run string, p Package) (Record, error) {
	c, err := s.run(ctx, run)
	if err != nil {
		return Record{}, err
	}
	ctx = runContext(ctx, c)
	contracts, err := s.environment.ContractsForRun(ctx, run)
	if err != nil || !dependenciesMatch(p, contracts) {
		return Record{}, ErrAccess
	}
	if _, err = Validate(ctx, p, s.owner, s.verifier, s.contracts); err != nil {
		return Record{}, err
	}
	source := map[string]any{"kind": "agent_proposal", "run_id": c.RunID, "task_id": c.TaskID, "task_revision": c.TaskRevision}
	ctx = context.WithValue(ctx, proposalAuthorityKey{}, c)
	r, err := s.stage(ctx, p, source)
	if err != nil {
		return r, err
	}
	if _, err = s.test(ctx, p.Digest, source); err != nil {
		return r, err
	}
	// Lack of a correction grant leaves a tested candidate. It never changes
	// active behavior or installs a new authority.
	if promoted, e := s.autoPromote(ctx, c, p.Digest); e == nil {
		return promoted, nil
	}
	return s.Get(ctx, p.Digest)
}

// ProposeSource retains the original manifest source as a deletion dependency.
// Tool arguments contain only exact references; prose never enters immutable
// action/approval bindings. Prior source identity alone grants no access.
func (s *Store) ProposeSource(ctx context.Context, run string, ref catalog.SourceRef) (Record, error) {
	if s.sources == nil || ref.ID == "" || ref.Revision < 1 || ref.AccessRevision < 1 || !digest.MatchString(ref.Hash) {
		return Record{}, ErrManifest
	}
	c, err := s.run(ctx, run)
	if err != nil {
		return Record{}, err
	}
	ctx = runContext(ctx, c)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.checkRunTx(ctx, tx, c); err != nil {
		return Record{}, err
	}
	rows, err := s.sources.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read")
	if err != nil || len(rows) != 1 {
		return Record{}, ErrAccess
	}
	p, err := Parse([]byte(rows[0].Body))
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return s.Propose(context.WithValue(ctx, proposalSourceKey{}, ref), run, p)
}
func (s *Store) autoPromote(ctx context.Context, c actionauth.ContextSnapshot, hash string) (Record, error) {
	if c.Authority == nil {
		return Record{}, ErrAccess
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.checkRunTx(ctx, tx, c); err != nil {
		return Record{}, err
	}
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return Record{}, err
	}
	if _, _, err = actionauth.CheckAssignmentTx(ctx, tx, *c.Authority, at); err != nil {
		return Record{}, err
	}
	r, err := s.metadata(ctx, tx, hash)
	if err != nil {
		return r, err
	}
	var parent string
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(active_digest,''),generation FROM skill_catalog WHERE skill_id=$1`, r.ID).Scan(&parent, &generation); err != nil || parent == "" {
		return r, ErrConflict
	}
	old, err := s.metadata(ctx, tx, parent)
	if err != nil || old.shape != r.shape {
		return r, ErrConflict
	}
	// Lock the complete union in catalog order before reading either body. Two
	// corrections with different derived roots must not lock shared parents in
	// opposite orders. Publication still needs current run/source authority.
	if _, err = s.sources.AccessTx(ctx, tx, []catalog.SourceRef{r.BodySource, old.BodySource}, "memory_read"); err != nil {
		return r, ErrUnavailable
	}
	if r, err = s.read(ctx, tx, hash); err != nil {
		return r, err
	}
	if _, err = s.read(ctx, tx, parent); err != nil {
		return r, err
	}
	// Both source graphs are validated before acquiring catalog/state locks.
	var currentParent string
	var currentGeneration int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(active_digest,''),generation FROM skill_catalog WHERE skill_id=$1 FOR UPDATE`, r.ID).Scan(&currentParent, &currentGeneration); err != nil || currentParent != parent || currentGeneration != generation {
		return r, ErrConflict
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return r, err
	}
	if _, _, err = actionauth.CheckAssignmentTx(ctx, tx, *c.Authority, at); err != nil {
		return r, err
	}
	rows, err := tx.Query(ctx, `SELECT expires_at FROM skill_correction_grants WHERE skill_id=$1 AND parent_digest=$2 AND owner_id=$3 AND class='descriptive_correction_v1' AND mandate_id=$4 AND mandate_revision=$5 AND purpose=$6 AND NOT revoked AND expires_at>$7 FOR SHARE`, r.ID, parent, c.OwnerID, c.Authority.MandateID, c.Authority.MandateRevision, c.Authority.Purpose, at)
	if err != nil {
		return r, err
	}
	grants := 0
	var grantExpiry time.Time
	for rows.Next() {
		grants++
		if err = rows.Scan(&grantExpiry); err != nil {
			rows.Close()
			return r, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || grants != 1 {
		return r, ErrAccess
	}
	// A grant lock can wait for concurrent revocation. Recheck database time
	// and the complete mandate after that wait, before changing active state.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil || !grantExpiry.After(at) {
		return r, ErrAccess
	}
	if _, _, err = actionauth.CheckAssignmentTx(ctx, tx, *c.Authority, at); err != nil {
		return r, err
	}
	r, err = s.activate(ctx, tx, hash, generation, map[string]any{"kind": "mandated_descriptive_correction", "run_id": c.RunID, "assignment": c.Authority})
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (s *Store) Usages(ctx context.Context, hash string) ([]Usage, error) {
	if _, err := s.actor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT run_id,task_id,task_revision,constraints_revision,digest,loaded_at FROM skill_usages WHERE digest=$1 ORDER BY loaded_at,run_id LIMIT 200`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Usage{}
	for rows.Next() {
		var u Usage
		if err = rows.Scan(&u.RunID, &u.TaskID, &u.TaskRevision, &u.ConstraintsRevision, &u.Digest, &u.LoadedAt); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}
