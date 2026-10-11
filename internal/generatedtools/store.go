package generatedtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool        *pgxpool.Pool
	owner       string
	profile     Profile
	sources     *catalog.Store
	environment RunEnvironment
	runtime     RuntimeClient
}

func NewStore(pool *pgxpool.Pool, owner string, profile Profile, sources *catalog.Store, environment RunEnvironment, runtime RuntimeClient) *Store {
	return &Store{pool: pool, owner: owner, profile: profile, sources: sources, environment: environment, runtime: runtime}
}
func encoded(v any) string { b, _ := json.Marshal(v); return string(b) }
func sourceRef(s catalog.Source) catalog.SourceRef {
	return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
func (s *Store) run(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	if s == nil || s.pool == nil || s.owner == "" || s.sources == nil || s.environment == nil || s.runtime == nil || s.profile.Validate() != nil {
		return actionauth.ContextSnapshot{}, ErrAccess
	}
	c, e := s.environment.GetActionContext(ctx, id)
	if e != nil || !c.ConverterLifecycleReady || c.RunID != id || c.OwnerID != s.owner || c.TaskID == "" || c.TaskStatus != "active" || c.TaskRevision != c.PinnedTaskRevision || c.TaskRevision < 1 || c.ConstraintsRevision < 1 || c.Authority == nil || c.LeaseID == "" || c.LeaseEpoch < 1 || !c.LeaseExpiresAt.After(time.Now()) {
		return c, ErrAccess
	}
	switch c.RunState {
	case "preparing", "model_running", "awaiting_model_resume", "RUN_STATE_PREPARING", "RUN_STATE_MODEL_RUNNING", "RUN_STATE_AWAITING_MODEL_RESUME", "tool_pending", "tool_running", "awaiting_approval", "RUN_STATE_TOOL_PENDING", "RUN_STATE_TOOL_RUNNING", "RUN_STATE_AWAITING_APPROVAL":
		return c, nil
	}
	return c, ErrAccess
}
func runContext(ctx context.Context, c actionauth.ContextSnapshot) context.Context {
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{})
	return ownership.WithToken(ctx, nil, ownership.Token{SessionKey: c.SessionKey, RunID: c.RunID, LeaseID: c.LeaseID, FenceEpoch: c.LeaseEpoch})
}

// Every publication follows ownership -> session -> run/task -> owner authority
// -> sorted sources -> converter lifecycle -> effect rows. No RPC or second
// connection is held under these locks; clock is rechecked before commit.
func (s *Store) guardRunTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot) (actionauth.Mandate, time.Time, error) {
	var lease, run, sessionOwner string
	var epoch int64
	var expires, now time.Time
	if tx.QueryRow(ctx, `SELECT lease_id,run_id,fence_epoch,expires_at FROM session_ownership WHERE session_key=$1 FOR SHARE`, c.SessionKey).Scan(&lease, &run, &epoch, &expires) != nil || lease != c.LeaseID || run != c.RunID || epoch != c.LeaseEpoch {
		return actionauth.Mandate{}, now, fmt.Errorf("converter ownership: %w", ErrAccess)
	}
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !expires.After(now) {
		return actionauth.Mandate{}, now, ErrAccess
	}
	if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR KEY SHARE`, c.SessionKey).Scan(&sessionOwner) != nil || sessionOwner != c.OwnerID {
		return actionauth.Mandate{}, now, ErrAccess
	}
	var pinned, status, state string
	var pinnedRevision int64
	if tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0),status,current_state FROM runs WHERE run_id=$1 FOR UPDATE`, c.RunID).Scan(&pinned, &pinnedRevision, &status, &state) != nil || pinned != c.TaskID || pinnedRevision != c.TaskRevision || status == "completed" || status == "failed" || status == "cancelled" || state != strings.ToLower(strings.TrimPrefix(c.RunState, "RUN_STATE_")) {
		return actionauth.Mandate{}, now, fmt.Errorf("converter current run: %w", ErrAccess)
	}
	var owner, taskStatus, policy string
	var revision, constraints int64
	if tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision,execution_policy FROM tasks WHERE task_id=$1 FOR SHARE`, c.TaskID).Scan(&owner, &taskStatus, &revision, &constraints, &policy) != nil || owner != c.OwnerID || taskStatus != "active" || revision != c.TaskRevision || constraints != c.ConstraintsRevision || policy != c.ExecutionPolicy {
		return actionauth.Mandate{}, now, ErrAccess
	}
	if e := actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, actionauth.EffectPrepare); e != nil {
		return actionauth.Mandate{}, now, e
	}
	a, e := actionauth.CheckTaskAuthorityTx(ctx, tx, c.TaskID, c.OwnerID, c.Authority, now)
	if e != nil || a.TaskID == "" {
		return actionauth.Mandate{}, now, fmt.Errorf("converter current task authority: %w", ErrAccess)
	}
	m, _, e := actionauth.CheckAssignmentTx(ctx, tx, a, now)
	if e != nil {
		return m, now, e
	}
	if e = s.sources.CheckRunSourcesTx(ctx, tx, c.RunID); e != nil {
		return m, now, e
	}
	if e = CheckContinuationRunTx(ctx, tx, s.sources, c.RunID); e != nil {
		return m, now, e
	}
	return m, now, nil
}
func (s *Store) permission(m actionauth.Mandate, n Need, now time.Time) (actionauth.ConverterPermission, error) {
	for _, p := range m.Converters {
		expected := ExpectedPermission(n, s.profile, p.Trials)
		if p == expected && actionauth.AllowsConverterAttachment(m, expected, now) {
			return p, nil
		}
	}
	return actionauth.ConverterPermission{}, ErrAccess
}
func (s *Store) metadataTx(ctx context.Context, tx pgx.Tx, hash string) (Record, error) {
	var r Record
	var body, need, shape []byte
	e := tx.QueryRow(ctx, `SELECT v.owner_id,v.converter_id,v.digest,v.version,v.need_id,v.need_hash,v.source_ref_json,n.source_ref_json,v.profile_pin,c.generation,st.status,COALESCE(c.active_digest=v.digest,false),v.created_at,v.permission_shape_json FROM converter_versions v JOIN converter_needs n ON n.owner_id=v.owner_id AND n.need_id=v.need_id JOIN converter_catalog c ON c.owner_id=v.owner_id AND c.converter_id=v.converter_id JOIN converter_states st ON st.owner_id=v.owner_id AND st.digest=v.digest WHERE v.owner_id=$1 AND v.digest=$2`, s.owner, hash).Scan(&r.OwnerID, &r.ID, &r.Digest, &r.Version, &r.NeedID, &r.NeedHash, &body, &need, &r.ProfilePin, &r.Generation, &r.Status, &r.Active, &r.CreatedAt, &shape)
	if e != nil || json.Unmarshal(body, &r.BodySource) != nil || json.Unmarshal(need, &r.NeedSource) != nil || json.Unmarshal(shape, &r.Shape) != nil || r.Shape.Validate() != nil || r.Shape.RuntimePin != r.ProfilePin || r.Shape.ConverterID != r.ID {
		return r, ErrAccess
	}
	return r, nil
}
func needFrom(source catalog.Source) (Need, error) {
	var n Need
	if Decode([]byte(source.Body), &n) != nil || n.Validate() != nil {
		return n, ErrPackage
	}
	return n, nil
}
func sourcesByID(rows []catalog.Source) map[string]catalog.Source {
	byID := map[string]catalog.Source{}
	for _, s := range rows {
		byID[s.ID] = s
	}
	return byID
}
func (s *Store) packageTx(ctx context.Context, tx pgx.Tx, r Record, extras ...catalog.SourceRef) (Package, Need, map[string]catalog.Source, error) {
	refs := []catalog.SourceRef{r.BodySource, r.NeedSource}
	for _, ref := range extras {
		found := false
		for _, old := range refs {
			if old.ID == ref.ID {
				if old != ref {
					return Package{}, Need{}, nil, ErrAccess
				}
				found = true
			}
		}
		if !found {
			refs = append(refs, ref)
		}
	}
	rows, e := s.sources.AccessTx(ctx, tx, refs, "memory_read")
	if e != nil {
		return Package{}, Need{}, nil, e
	}
	byID := sourcesByID(rows)
	p, e := Parse([]byte(byID[r.BodySource.ID].Body))
	if e != nil || p.Digest != r.Digest {
		return p, Need{}, nil, ErrPackage
	}
	n, e := needFrom(byID[r.NeedSource.ID])
	if e != nil || Hash(n) != r.NeedHash || p.Manifest.ID != r.ID || p.Manifest.Version != r.Version || p.Manifest.NeedID != r.NeedID || n.ID != r.NeedID || p.Manifest.Dependencies[0] != s.profile.Dependency {
		return p, n, nil, ErrAccess
	}
	c, e := s.ownedSnapshotTx(ctx, tx)
	if e != nil {
		return p, n, nil, e
	}
	if e = s.checkReturnedPackageTx(ctx, tx, c, r); e != nil {
		return p, n, nil, e
	}
	return p, n, byID, nil
}
