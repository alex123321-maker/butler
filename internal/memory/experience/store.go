package experience

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5"
	"time"
)

type ToolCatalog interface {
	ListToolsForRun(context.Context, string) ([]*toolbrokerv1.ToolContract, error)
}
type Store struct {
	Sources         *catalog.Store
	EnvironmentHash string
	Tools           ToolCatalog
}
type Prepared struct {
	fingerprint string
	tools       map[string]skills.Dependency
}

func New(sources *catalog.Store, environmentHash string, tools ToolCatalog) (*Store, error) {
	if sources == nil || !hexHash.MatchString(environmentHash) || tools == nil {
		return nil, ErrEvidence
	}
	return &Store{sources, environmentHash, tools}, nil
}

// Prepare obtains the current permitted Broker contracts before any owning SQL
// transaction. Missing/unknown versions are inapplicable, never a wildcard.
func (s *Store) Prepare(ctx context.Context, run string) (Prepared, error) {
	if s == nil || run == "" || s.Tools == nil {
		return Prepared{}, ErrEvidence
	}
	contracts, err := s.Tools.ListToolsForRun(ctx, run)
	if err != nil || len(contracts) > 256 {
		return Prepared{}, ErrEvidence
	}
	p := Prepared{s.EnvironmentHash, map[string]skills.Dependency{}}
	for _, c := range contracts {
		h, err := actionauth.ContractHash(c)
		if err != nil || c.GetStatus() != "active" || c.GetContractRevision() == "" {
			continue
		}
		if _, ok := p.tools[c.ToolName]; ok {
			return Prepared{}, ErrEvidence
		}
		p.tools[c.ToolName] = skills.Dependency{Tool: c.ToolName, ContractHash: h, ContractRevision: c.ContractRevision}
	}
	return p, nil
}

// Seal is called only by the owning typed task/effect adapter. Model/HTTP input
// cannot supply Observation or parents. Exact retries reuse the original seal;
// later explicit evidence creates a new immutable case, retaining unknowns.
func (s *Store) Seal(ctx context.Context, o Observation, parents []catalog.SourceRef) (catalog.SourceRef, error) {
	a, ok := owneridentity.FromContext(ctx)
	if s == nil || !ok || a.OwnerID != o.OwnerID || a.DeliveryID == "" || o.Validate() != nil || o.Environment.Fingerprint != s.EnvironmentHash || len(parents) == 0 {
		return catalog.SourceRef{}, ErrEvidence
	}
	tx, err := s.Sources.Pool().Begin(ctx)
	if err != nil {
		return catalog.SourceRef{}, err
	}
	defer tx.Rollback(ctx)
	// Session/task locks precede source rows, matching publication/deletion.
	var session, owner string
	var revision, constraints int64
	err = tx.QueryRow(ctx, `SELECT r.session_key,t.owner_id,t.revision,t.constraints_revision FROM runs r JOIN tasks t USING(task_id) WHERE r.run_id=$1 AND t.task_id=$2 AND r.task_revision=$3`, o.RunID, o.TaskID, o.TaskRevision).Scan(&session, &owner, &revision, &constraints)
	if err != nil || owner != o.OwnerID || constraints != o.ConstraintsRevision {
		return catalog.SourceRef{}, ErrEvidence
	}
	if _, err = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, session); err != nil {
		return catalog.SourceRef{}, err
	}
	var status, completion string
	if err = tx.QueryRow(ctx, `SELECT owner_id,revision,constraints_revision,status,COALESCE(completion_result_id,'') FROM tasks WHERE task_id=$1 FOR SHARE`, o.TaskID).Scan(&owner, &revision, &constraints, &status, &completion); err != nil || owner != o.OwnerID || constraints != o.ConstraintsRevision || (status != "active" && status != "completed") || revision != o.TaskRevision && (status != "completed" || revision != o.TaskRevision+1 || o.OwnerVerdict != "verified" || completion != o.OwnerResultID) {
		return catalog.SourceRef{}, ErrEvidence
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, o.TaskID, o.OwnerID, nil, time.Now()); err != nil {
		return catalog.SourceRef{}, err
	}
	if session != o.SessionKey {
		return catalog.SourceRef{}, ErrEvidence
	}
	// Bind copied data to exact owning rows before any new snapshot is created.
	// The captured head is retained for audit; unrelated changes to it do not
	// stand in for a change to one of these actual observations.
	for _, pin := range o.ToolPins {
		var digest string
		// Follow the writer's primary-tool -> metadata-head order. A separate
		// statement after obtaining the primary lock sees a completed writer.
		var id string
		if err = tx.QueryRow(ctx, `SELECT tool_call_id FROM tool_calls WHERE tool_call_id=$1 AND run_id=$2 FOR SHARE`, pin.ID, pin.RunID).Scan(&id); err != nil {
			return catalog.SourceRef{}, catalog.ErrStale
		}
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT row_hash,revision FROM memory_experience_tool_heads WHERE tool_call_id=$1 AND run_id=$2 AND alive`, pin.ID, pin.RunID).Scan(&digest, &revision); err != nil || digest != pin.Hash || revision != pin.Revision {
			return catalog.SourceRef{}, catalog.ErrStale
		}
	}
	if _, err = s.Sources.AccessTx(ctx, tx, parents, "memory_job_create"); err != nil {
		return catalog.SourceRef{}, err
	}
	var prior string
	var digest string
	err = tx.QueryRow(ctx, `SELECT source_id,observation_hash FROM memory_experience_observations WHERE owner_id=$1 AND run_id=$2 AND result_id=$3 AND observation_hash=$4`, o.OwnerID, o.RunID, o.ResultID, hash(o)).Scan(&prior, &digest)
	if err == nil {
		if digest != hash(o) {
			return catalog.SourceRef{}, catalog.ErrStale
		}
		ss, err := s.Sources.AccessTx(ctx, tx, []catalog.SourceRef{{ID: prior}}, "memory_job_create")
		if err != nil || len(ss) != 1 {
			return catalog.SourceRef{}, ErrEvidence
		}
		return ref(ss[0]), tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return catalog.SourceRef{}, err
	}
	body, _ := json.Marshal(o)
	src, err := s.Sources.CreateDerivedTx(ctx, tx, parents, "memory_job_create", "Typed observed task experience", string(body), json.RawMessage(`{"format":"butler.episodic.observation.v1"}`))
	if err != nil {
		return catalog.SourceRef{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO memory_experience_observations(source_id,source_revision,owner_id,observation_hash,task_id,run_id,result_id,session_key,transcript_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, src.ID, src.Revision, o.OwnerID, hash(o), o.TaskID, o.RunID, o.ResultID, o.SessionKey, o.TranscriptRevision)
	if err != nil {
		return catalog.SourceRef{}, err
	}
	for _, pin := range o.ToolPins {
		if _, err = tx.Exec(ctx, `INSERT INTO memory_experience_tool_pins(observation_source_id,tool_call_id,run_id,row_hash,row_revision)VALUES($1,$2,$3,$4,$5)`, src.ID, pin.ID, pin.RunID, pin.Hash, pin.Revision); err != nil {
			return catalog.SourceRef{}, err
		}
	}
	return ref(src), tx.Commit(ctx)
}

// EligibleTx runs only after full Catalog access. All copy-bearing text remains
// in that graph; these metadata checks add applicability, never authority.
// Ordinary sources pass through. Typed input snapshots never masquerade as a
// recipe. An unaccepted, negatively reviewed or incompatible lesson is hidden.
func (s *Store) EligibleTx(ctx context.Context, tx pgx.Tx, src catalog.Source, p Prepared, pin bool) (bool, error) {
	var observation bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_observations WHERE source_id=$1)`, src.ID).Scan(&observation); err != nil {
		return false, err
	}
	if observation {
		return false, nil
	}
	var env, digest, owner, claim, status string
	var revision int64
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT l.environment_hash,l.lesson_hash,l.owner_id,l.claim_id,l.source_revision,l.dependencies_json,c.status FROM memory_experience_lessons l JOIN memory_claims c ON c.claim_id=l.claim_id WHERE l.source_id=$1 FOR SHARE OF l,c`, src.ID).Scan(&env, &digest, &owner, &claim, &revision, &raw, &status)
	if err == pgx.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if s == nil || p.fingerprint == "" || env != p.fingerprint || env != s.EnvironmentHash || status != "confirmed" || revision != src.Revision || owner != src.OwnerID {
		return false, nil
	}
	var negative bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_feedback WHERE source_id=$1 AND verdict IN ('negative','misapplied','correction_required'))`, src.ID).Scan(&negative); err != nil {
		return false, err
	}
	if negative {
		return false, nil
	}
	var deps []skills.Dependency
	if json.Unmarshal(raw, &deps) != nil || len(deps) == 0 || len(deps) > MaxActions {
		return false, ErrEvidence
	}
	for _, d := range deps {
		if p.tools[d.Tool] != d {
			return false, nil
		}
	}
	if err = skills.CheckInstalledDependenciesTx(ctx, tx, owner, deps); err != nil {
		return false, nil
	}
	var lesson Lesson
	if skills.Decode([]byte(src.Body), &lesson) != nil || lesson.Format != Format || hash(lesson) != digest || lesson.Observation.Validate() != nil || lesson.Verification != verification(lesson.Observation) || lesson.InstructionAuthority {
		return false, ErrEvidence
	}
	if pin {
		t, ok := ownership.TokenFromContext(ctx)
		if !ok {
			return false, ErrEvidence
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM memory_experience_pins WHERE run_id=$1`, t.RunID).Scan(&count); err != nil {
			return false, err
		}
		if count >= 8 {
			var exists bool
			if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_pins WHERE run_id=$1 AND source_id=$2)`, t.RunID, src.ID).Scan(&exists) != nil || !exists {
				return false, ErrEvidence
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO memory_experience_pins(run_id,source_id,environment_hash,lesson_hash)VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, t.RunID, src.ID, env, digest)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// Feedback preserves the owner's explicit observation, including unknown. It
// neither marks a task successful nor erases contradictory/negative feedback.
func (s *Store) Feedback(ctx context.Context, source, run, verdict string) error {
	a, ok := owneridentity.FromContext(ctx)
	if s == nil || !ok || a.DeliveryID == "" || !text(run, 160) {
		return ErrEvidence
	}
	switch verdict {
	case "useful", "misapplied", "correction_required", "negative", "unknown":
	default:
		return ErrEvidence
	}
	tx, err := s.Sources.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = s.Sources.AccessTx(ctx, tx, []catalog.SourceRef{{ID: source}}, "memory_read"); err != nil {
		return err
	}
	var owner string
	if tx.QueryRow(ctx, `SELECT owner_id FROM memory_experience_lessons WHERE source_id=$1 FOR UPDATE`, source).Scan(&owner) != nil || owner != a.OwnerID {
		return ErrEvidence
	}
	var belongs bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1 AND s.user_id=$2)`, run, a.OwnerID).Scan(&belongs) != nil || !belongs {
		return ErrEvidence
	}
	id := hash([]string{a.OwnerID, source, run})
	tag, err := tx.Exec(ctx, `INSERT INTO memory_experience_feedback(feedback_id,owner_id,source_id,run_id,verdict) VALUES($1,$2,$3,$4,$5)ON CONFLICT DO NOTHING`, id, a.OwnerID, source, run, verdict)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var prior string
		if tx.QueryRow(ctx, `SELECT verdict FROM memory_experience_feedback WHERE feedback_id=$1`, id).Scan(&prior) != nil || prior != verdict {
			return catalog.ErrStale
		}
	}
	return tx.Commit(ctx)
}

// CheckPinnedTx adds current environment/feedback/dependency proof before every
// subsequent provider/tool/final release. It performs no Broker RPC under locks.
func (s *Store) CheckPinnedTx(ctx context.Context, tx pgx.Tx, run string) error {
	rows, err := tx.Query(ctx, `SELECT l.source_id,l.source_revision,l.environment_hash,l.lesson_hash,l.dependencies_json,l.owner_id,c.status,p.environment_hash,p.lesson_hash FROM memory_experience_pins p JOIN memory_experience_lessons l USING(source_id) JOIN memory_claims c ON c.claim_id=l.claim_id WHERE p.run_id=$1 ORDER BY l.source_id FOR SHARE OF l,c`, run)
	if err != nil {
		return err
	}
	type pin struct {
		id, env, digest, owner, status, pEnv, pDigest string
		revision                                      int64
		raw                                           []byte
	}
	pins := []pin{}
	for rows.Next() {
		var p pin
		if err = rows.Scan(&p.id, &p.revision, &p.env, &p.digest, &p.raw, &p.owner, &p.status, &p.pEnv, &p.pDigest); err != nil {
			rows.Close()
			return err
		}
		pins = append(pins, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(pins) > 8 {
		return ErrEvidence
	}
	for _, p := range pins {
		if s == nil || s.EnvironmentHash == "" || p.env != s.EnvironmentHash || p.env != p.pEnv || p.digest != p.pDigest || p.status != "confirmed" {
			return ErrEvidence
		}
		var negative bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_feedback WHERE source_id=$1 AND verdict IN ('negative','misapplied','correction_required'))`, p.id).Scan(&negative) != nil || negative {
			return ErrEvidence
		}
		var deps []skills.Dependency
		if json.Unmarshal(p.raw, &deps) != nil || len(deps) == 0 {
			return ErrEvidence
		}
		if err = skills.CheckInstalledDependenciesTx(ctx, tx, p.owner, deps); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CheckPinned(ctx context.Context, run string) error {
	if s == nil || s.Sources == nil {
		return ErrEvidence
	}
	var count int
	if s.Sources.Pool().QueryRow(ctx, `SELECT count(*) FROM memory_experience_pins WHERE run_id=$1`, run).Scan(&count) != nil || count > 8 {
		return ErrEvidence
	}
	if count == 0 {
		return nil
	}
	p, err := s.Prepare(ctx, run)
	if err != nil {
		return err
	}
	tx, err := s.Sources.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.Sources.CheckRunSourcesTx(ctx, tx, run); err != nil {
		return err
	}
	if err = s.CheckPinnedTx(ctx, tx, run); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT l.dependencies_json FROM memory_experience_pins p JOIN memory_experience_lessons l USING(source_id) WHERE p.run_id=$1`, run)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		var deps []skills.Dependency
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &deps) != nil {
			rows.Close()
			return ErrEvidence
		}
		for _, d := range deps {
			if p.tools[d.Tool] != d {
				rows.Close()
				return ErrEvidence
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Ancestors returns only exact lesson identities in the authoritative full
// Catalog graph. It prevents legacy admission by omission of lesson_sources.
func (s *Store) Ancestors(ctx context.Context, roots []catalog.SourceRef) ([]catalog.SourceRef, error) {
	if s == nil || len(roots) > 64 {
		return nil, ErrEvidence
	}
	tx, err := s.Sources.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	graph, err := s.Sources.CanonicalSourcesTx(ctx, tx, roots)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, r := range graph {
		ids = append(ids, r.ID)
	}
	rows, err := tx.Query(ctx, `SELECT s.source_id,l.source_revision,s.access_revision,r.content_hash FROM memory_experience_lessons l JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=l.source_id AND r.revision=l.source_revision WHERE l.source_id=ANY($1) ORDER BY l.source_id`, ids)
	if err != nil {
		return nil, err
	}
	out := []catalog.SourceRef{}
	for rows.Next() {
		var r catalog.SourceRef
		if err = rows.Scan(&r.ID, &r.Revision, &r.AccessRevision, &r.Hash); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

// VerifyPromotion is supplemental #19 validation, never activation. Every case
// must be independently owner verified and correspond to distinct task/run
// identities. All failures/unknowns remain in the lesson and disqualify recipe
// promotion; a useful/absent owner comment cannot override this proof.
func (s *Store) VerifyPromotion(ctx context.Context, owner string, lessons []catalog.SourceRef, proofs []skills.SourceProof, deps []skills.Dependency) ([]catalog.SourceRef, error) {
	if s == nil || len(lessons) < 2 || len(lessons) > MaxCases || len(proofs) < 2 {
		return nil, ErrEvidence
	}
	p := Prepared{s.EnvironmentHash, map[string]skills.Dependency{}}
	for _, d := range deps {
		p.tools[d.Tool] = d
	}
	tx, err := s.Sources.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	sources, err := s.Sources.AccessTx(ctx, tx, lessons, "memory_read")
	if err != nil || len(sources) != len(lessons) {
		return nil, ErrEvidence
	}
	tasks := map[string]bool{}
	runs := map[string]bool{}
	out := []catalog.SourceRef{}
	for _, src := range sources {
		if src.OwnerID != owner {
			return nil, ErrEvidence
		}
		allowed, err := s.EligibleTx(ctx, tx, src, p, false)
		if err != nil || !allowed {
			return nil, ErrEvidence
		}
		var l Lesson
		if skills.Decode([]byte(src.Body), &l) != nil || l.Format != Format || l.Verification != "owner_verified_with_observed_actions" {
			return nil, ErrEvidence
		}
		o := l.Observation
		if tasks[o.TaskID] || runs[o.RunID] {
			return nil, ErrEvidence
		}
		tasks[o.TaskID] = true
		runs[o.RunID] = true
		for _, a := range o.Actions {
			if a.Effect != actionauth.StateConfirmed || a.Failure != "" {
				return nil, ErrEvidence
			}
		}
		if len(o.Unknowns) != 0 || len(o.Failures) != 0 {
			return nil, ErrEvidence
		}
		matched := false
		for _, proof := range proofs {
			if proof.TaskID == o.TaskID && proof.TaskRevision == o.TaskRevision && proof.ResultID == o.OwnerResultID && proof.OwnerID == owner && proof.Verifier == "owner" && proof.Verdict == "verified" && proof.Failures == 0 && len(proof.ConfirmedActions) > 0 {
				actions := map[string]bool{}
				contracts := map[skills.Dependency]bool{}
				for _, id := range proof.ConfirmedActions {
					actions[id] = true
				}
				for _, d := range proof.ConfirmedContracts {
					contracts[d] = true
				}
				matched = true
				for _, a := range o.Actions {
					if !actions[a.ID] || !contracts[skills.Dependency{Tool: a.Tool, ContractHash: a.ContractHash, ContractRevision: a.ContractRevision}] {
						matched = false
					}
				}
			}
		}
		if !matched {
			return nil, ErrEvidence
		}
		out = append(out, ref(src))
	}
	return out, tx.Commit(ctx)
}
