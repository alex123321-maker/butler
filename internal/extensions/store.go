package extensions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool     *pgxpool.Pool
	ownerID  string
	profiles map[string]Profile
	budgets  *budget.Controller
}

func NewStore(pool *pgxpool.Pool, ownerID string, profiles map[string]Profile, budgets *budget.Controller) *Store {
	copyProfiles := make(map[string]Profile, len(profiles))
	for id, profile := range profiles {
		copyProfiles[id] = profile
	}
	return &Store{pool: pool, ownerID: ownerID, profiles: copyProfiles, budgets: budgets}
}
func (s *Store) actor(ctx context.Context) (map[string]string, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s.ownerID == "" || a.OwnerID != s.ownerID {
		return nil, ErrAccess
	}
	return map[string]string{"author": a.OwnerID, "channel": a.Channel, "delivery_id": a.DeliveryID}, nil
}
func (s *Store) profile(p Package) (Profile, error) {
	profile, ok := s.profiles[p.Manifest.Runtime.Profile]
	if !ok || profile.Target == "" || !identifier.MatchString(p.Manifest.Runtime.Profile) {
		return Profile{}, ErrUnavailable
	}
	for _, dependency := range p.Manifest.Dependencies {
		if dependency == profile.Dependency {
			return profile, nil
		}
	}
	return Profile{}, ErrUnavailable
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("extension secure identity unavailable")
	}
	return hex.EncodeToString(b[:])
}
func encoded(v any) string { b, _ := json.Marshal(v); return string(b) }
func event(ctx context.Context, tx pgx.Tx, r Record, kind string, source map[string]string) error {
	_, err := tx.Exec(ctx, `INSERT INTO extension_events(extension_id,manifest_hash,generation,kind,source_json) VALUES($1,$2,$3,$4,$5::jsonb)`, r.ID, r.ManifestHash, r.Generation, kind, encoded(source))
	return err
}
func (s *Store) Stage(ctx context.Context, p Package) (Record, error) {
	source, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	// Reparse at the durable boundary; callers cannot construct a forged Package.
	p, err = Parse(bytes.NewReader(p.Raw))
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO extension_versions(manifest_hash,extension_id,version,source_uri,manifest_text) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, p.Digest, p.Manifest.ID, p.Manifest.Version, p.Manifest.Source.URI, string(p.Raw))
	if err != nil {
		return Record{}, err
	}
	var digest string
	err = tx.QueryRow(ctx, `SELECT manifest_hash FROM extension_versions WHERE extension_id=$1 AND version=$2`, p.Manifest.ID, p.Manifest.Version).Scan(&digest)
	if err != nil || digest != p.Digest {
		return Record{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO extension_installations(extension_id,owner_id,manifest_hash,generation,status) VALUES($1,$2,$3,1,'staged') ON CONFLICT DO NOTHING`, p.Manifest.ID, s.ownerID, p.Digest)
	if err != nil {
		return Record{}, err
	}
	r, err := s.read(ctx, tx, p.Manifest.ID, "FOR UPDATE OF i")
	if err != nil || r.OwnerID != s.ownerID {
		return Record{}, ErrAccess
	}
	versionEvent := r
	versionEvent.ManifestHash = p.Digest
	if err = event(ctx, tx, versionEvent, "staged", source); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return r, nil
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ManifestByHash reads immutable package history for trusted provenance readers.
// It grants no installation, readiness, task access or execution permission.
func (s *Store) ManifestByHash(ctx context.Context, digest string) (Package, error) {
	if !digestName.MatchString(digest) {
		return Package{}, ErrUnavailable
	}
	var raw string
	if err := s.pool.QueryRow(ctx, `SELECT manifest_text FROM extension_versions WHERE manifest_hash=$1`, digest).Scan(&raw); err != nil {
		return Package{}, ErrUnavailable
	}
	p, err := Parse(strings.NewReader(raw))
	if err != nil || p.Digest != digest {
		return Package{}, ErrUnavailable
	}
	return p, nil
}

func (s *Store) read(ctx context.Context, q querier, id, lock string) (Record, error) {
	var r Record
	var config, refs, tools, raw string
	var pin *string
	err := q.QueryRow(ctx, `SELECT i.extension_id,i.owner_id,i.manifest_hash,i.generation,i.status,i.config_json::text,i.credential_refs_json::text,i.allowed_tools_json::text,i.health,i.health_reason,i.healthy_until,v.manifest_text,r.profile_pin FROM extension_installations i JOIN extension_versions v ON v.manifest_hash=i.manifest_hash LEFT JOIN extension_reviews r ON r.manifest_hash=i.manifest_hash AND r.owner_id=i.owner_id WHERE i.extension_id=$1 `+lock, id).Scan(&r.ID, &r.OwnerID, &r.ManifestHash, &r.Generation, &r.Status, &config, &refs, &tools, &r.Health, &r.HealthReason, &r.HealthyUntil, &raw, &pin)
	if err != nil {
		return r, ErrUnavailable
	}
	r.Package, err = Parse(strings.NewReader(raw))
	if err != nil || r.Package.Digest != r.ManifestHash {
		return r, ErrUnavailable
	}
	if json.Unmarshal([]byte(config), &r.Configuration) != nil || json.Unmarshal([]byte(refs), &r.CredentialRefs) != nil || json.Unmarshal([]byte(tools), &r.AllowedTools) != nil {
		return r, ErrUnavailable
	}
	profile, err := s.profile(r.Package)
	r.Reviewed = err == nil && pin != nil && *pin == hashJSON(profile)
	missing, err := configuration(r.Package.Manifest, r.Configuration, r.CredentialRefs)
	r.Available = r.Status == "enabled" && r.Reviewed && r.Health == "healthy" && r.HealthyUntil != nil && r.HealthyUntil.After(time.Now().UTC()) && err == nil && len(missing) == 0 && selectedTools(r.Package.Manifest, r.AllowedTools) == nil
	if len(missing) > 0 {
		r.HealthReason = "missing_settings:" + strings.Join(missing, ",")
	}
	return r, nil
}
func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	return s.read(ctx, s.pool, id, "")
}
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if _, err := s.actor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT extension_id FROM extension_installations WHERE owner_id=$1 ORDER BY extension_id`, s.ownerID)
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
	result := make([]Record, 0, len(ids))
	for _, id := range ids {
		r, e := s.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		result = append(result, r)
	}
	return result, nil
}
func (s *Store) Review(ctx context.Context, id, digest string) error {
	source, err := s.actor(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, id, "FOR UPDATE OF i")
	if err != nil || r.OwnerID != s.ownerID {
		return ErrAccess
	}
	var raw string
	err = tx.QueryRow(ctx, `SELECT manifest_text FROM extension_versions WHERE extension_id=$1 AND manifest_hash=$2`, id, digest).Scan(&raw)
	if err != nil {
		return ErrUnavailable
	}
	p, err := Parse(strings.NewReader(raw))
	if err != nil {
		return err
	}
	profile, err := s.profile(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO extension_reviews(manifest_hash,owner_id,profile_pin,source_json) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT DO NOTHING`, digest, s.ownerID, hashJSON(profile), encoded(source))
	if err != nil {
		return err
	}
	var pin string
	err = tx.QueryRow(ctx, `SELECT profile_pin FROM extension_reviews WHERE manifest_hash=$1 AND owner_id=$2`, digest, s.ownerID).Scan(&pin)
	if err != nil || pin != hashJSON(profile) {
		return ErrConflict
	}
	r.ManifestHash = digest
	if err = event(ctx, tx, r, "reviewed", source); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Configure selects an exact reviewed version and disables it before testing.
// Updates and rollbacks share this path; neither restores an old dispatch grant.
func (s *Store) Configure(ctx context.Context, id, digest string, generation int64, config map[string]string, refs map[string]actionauth.CredentialRef, tools []string) (Record, error) {
	source, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, id, "FOR UPDATE OF i")
	if err != nil {
		return r, err
	}
	if r.OwnerID != s.ownerID || r.Generation != generation {
		return r, ErrConflict
	}
	var raw, pin string
	err = tx.QueryRow(ctx, `SELECT v.manifest_text,r.profile_pin FROM extension_versions v JOIN extension_reviews r ON r.manifest_hash=v.manifest_hash WHERE v.extension_id=$1 AND v.manifest_hash=$2 AND r.owner_id=$3`, id, digest, s.ownerID).Scan(&raw, &pin)
	if err != nil {
		return r, ErrUnavailable
	}
	p, err := Parse(strings.NewReader(raw))
	if err != nil {
		return r, err
	}
	profile, err := s.profile(p)
	if err != nil || pin != hashJSON(profile) {
		return r, ErrUnavailable
	}
	if _, err = configuration(p.Manifest, config, refs); err != nil {
		return r, err
	}
	if err = selectedTools(p.Manifest, tools); err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `UPDATE extension_probes SET state='unknown',finished_at=clock_timestamp() WHERE extension_id=$1 AND state='executing'`, id); err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, `UPDATE extension_installations SET manifest_hash=$2,generation=generation+1,status='disabled',config_json=$3::jsonb,credential_refs_json=$4::jsonb,allowed_tools_json=$5::jsonb,health='unknown',health_reason='not_validated',healthy_until=NULL,updated_at=clock_timestamp() WHERE extension_id=$1`, id, digest, encoded(config), encoded(refs), encoded(tools))
	if err != nil {
		return r, err
	}
	r, err = s.read(ctx, tx, id, "")
	if err != nil {
		return r, err
	}
	if err = event(ctx, tx, r, "configured", source); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (s *Store) SetEnabled(ctx context.Context, id string, generation int64, enable bool) (Record, error) {
	source, err := s.actor(ctx)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, id, "FOR UPDATE OF i")
	if err != nil {
		return r, err
	}
	if r.OwnerID != s.ownerID || r.Generation != generation {
		return r, ErrConflict
	}
	state, kind := "disabled", "disabled"
	if enable {
		missing, e := configuration(r.Package.Manifest, r.Configuration, r.CredentialRefs)
		if e != nil || len(missing) > 0 || !r.Reviewed || r.Health != "healthy" || r.HealthyUntil == nil || !r.HealthyUntil.After(time.Now().UTC()) {
			return r, ErrUnavailable
		}
		state, kind = "enabled", "enabled"
	}
	if !enable {
		if _, err = tx.Exec(ctx, `UPDATE extension_probes SET state='unknown',finished_at=clock_timestamp() WHERE extension_id=$1 AND state='executing'`, id); err != nil {
			return r, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE extension_installations SET status=$2,generation=generation+1,updated_at=clock_timestamp() WHERE extension_id=$1`, id, state)
	if err != nil {
		return r, err
	}
	r, err = s.read(ctx, tx, id, "")
	if err != nil {
		return r, err
	}
	if err = event(ctx, tx, r, kind, source); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// BeginProbe reserves the immutable owner/admin allowances and persists the
// non-replayable attempt before egress. No credentials/body/redirects are used.
func (s *Store) BeginProbe(ctx context.Context, id string, generation int64) (Probe, Record, error) {
	source, err := s.actor(ctx)
	if err != nil {
		return Probe{}, Record{}, err
	}
	if s.budgets == nil || s.budgets.Ledger == nil {
		return Probe{}, Record{}, ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Probe{}, Record{}, err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, id, "FOR UPDATE OF i")
	if err != nil {
		return Probe{}, r, err
	}
	if r.OwnerID != s.ownerID || r.Generation != generation || !r.Reviewed {
		return Probe{}, r, ErrConflict
	}
	missing, err := configuration(r.Package.Manifest, r.Configuration, r.CredentialRefs)
	if err != nil || len(missing) > 0 {
		return Probe{}, r, ErrUnavailable
	}
	target, err := readinessURL(r)
	if err != nil {
		return Probe{}, r, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Probe{}, r, err
	}
	// The installation lock serializes admission. Parallel validations cannot
	// overwrite newer health, and an interrupted attempt is never replayed.
	if _, err = tx.Exec(ctx, `UPDATE extension_probes SET state='unknown',finished_at=clock_timestamp() WHERE extension_id=$1 AND state='executing' AND started_at<$2::timestamptz-interval '1 minute'`, id, now); err != nil {
		return Probe{}, r, err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM extension_probes WHERE extension_id=$1 AND state='executing')`, id).Scan(&active); err != nil {
		return Probe{}, r, err
	}
	if active {
		return Probe{}, r, ErrConflict
	}
	day := now.UTC().Format("2006-01-02")
	deadline := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	ids := []string{}
	for _, spec := range []budget.ScopeSpec{{Kind: "owner_day", OwnerID: s.ownerID, SubjectID: day, Limits: s.budgets.Policy.Day, Deadline: deadline}, {Kind: "conversation", OwnerID: s.ownerID, SubjectID: "extension-admin:" + day, Limits: s.budgets.Policy.Conversation, Deadline: deadline}} {
		scope, e := s.budgets.Ledger.EnsureTx(ctx, tx, spec)
		if e != nil {
			return Probe{}, r, e
		}
		ids = append(ids, scope.ID)
	}
	p := Probe{ID: randomID(), ExtensionID: id, ManifestHash: r.ManifestHash, Generation: r.Generation, ConfigHash: hashJSON(struct {
		Config map[string]string
		Refs   map[string]actionauth.CredentialRef
		Tools  []string
	}{r.Configuration, r.CredentialRefs, r.AllowedTools}), URL: target, State: "executing"}
	charge := budget.Units{ExternalCalls: 1, ActiveMillis: r.Package.Manifest.Limits.TimeoutMillis, EgressBytes: r.Package.Manifest.Limits.EgressBytes}
	if _, _, err = s.budgets.Ledger.ReserveTx(ctx, tx, budget.Request{OwnerID: s.ownerID, OperationID: "extension-probe:" + p.ID, ScopeIDs: ids, Charge: charge, CostKnown: true}); err != nil {
		return Probe{}, r, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO extension_probes(probe_id,extension_id,manifest_hash,generation,config_hash,state) VALUES($1,$2,$3,$4,$5,'executing')`, p.ID, id, p.ManifestHash, p.Generation, p.ConfigHash)
	if err != nil {
		return Probe{}, r, err
	}
	if err = event(ctx, tx, r, "probe_started", source); err != nil {
		return Probe{}, r, err
	}
	if _, err = tx.Exec(ctx, `UPDATE extension_installations SET health='unknown',health_reason='validation_pending',healthy_until=NULL WHERE extension_id=$1`, r.ID); err != nil {
		return Probe{}, r, err
	}
	return p, r, tx.Commit(ctx)
}
func (s *Store) FinishProbe(ctx context.Context, p Probe, ready bool, resultDigest string) error {
	if _, err := s.actor(ctx); err != nil {
		return err
	}
	if !digestName.MatchString(resultDigest) {
		return ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, p.ExtensionID, "FOR UPDATE OF i")
	if err != nil {
		return err
	}
	if r.OwnerID != s.ownerID || r.Generation != p.Generation || r.ManifestHash != p.ManifestHash {
		return ErrConflict
	}
	state, health, reason := "failed", "failed", "probe_failed"
	if ready {
		state, health, reason = "ready", "healthy", "ready"
	}
	tag, err := tx.Exec(ctx, `UPDATE extension_probes SET state=$2,result_json=$3::jsonb,finished_at=clock_timestamp() WHERE probe_id=$1 AND state='executing' AND extension_id=$4 AND manifest_hash=$5 AND generation=$6 AND config_hash=$7`, p.ID, state, encoded(map[string]string{"response_digest": resultDigest}), p.ExtensionID, p.ManifestHash, p.Generation, p.ConfigHash)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE extension_installations SET health=$2,health_reason=$3,healthy_until=CASE WHEN $2='healthy' THEN clock_timestamp()+interval '15 minutes' ELSE NULL END,updated_at=clock_timestamp() WHERE extension_id=$1`, r.ID, health, reason)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) RecoverProbes(ctx context.Context) error {
	// Close the cursor before taking per-installation locks so MaxConns2 remains
	// sufficient. Lock order matches admission/configuration/finish.
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT extension_id FROM extension_probes WHERE state='executing' AND started_at<clock_timestamp()-interval '1 minute' ORDER BY extension_id`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.recoverProbe(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recoverProbe(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := s.read(ctx, tx, id, "FOR UPDATE OF i")
	if err != nil {
		return err
	}
	var changed bool
	err = tx.QueryRow(ctx, `WITH abandoned AS (UPDATE extension_probes SET state='unknown',finished_at=clock_timestamp() WHERE extension_id=$1 AND state='executing' AND started_at<clock_timestamp()-interval '1 minute' RETURNING generation,manifest_hash) SELECT EXISTS(SELECT 1 FROM abandoned WHERE generation=$2 AND manifest_hash=$3)`, id, r.Generation, r.ManifestHash).Scan(&changed)
	if err != nil {
		return err
	}
	if changed {
		if _, err = tx.Exec(ctx, `UPDATE extension_installations SET health='unknown',health_reason='probe_unknown',healthy_until=NULL,updated_at=clock_timestamp() WHERE extension_id=$1`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) GrantTask(ctx context.Context, id string, access TaskAccess) error {
	source, err := s.actor(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner, status string
	var revision, constraints int64
	err = tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, access.TaskID).Scan(&owner, &status, &revision, &constraints)
	if err != nil || owner != s.ownerID || status != "active" || revision != access.TaskRevision || constraints != access.ConstraintsRevision || !access.ExpiresAt.After(time.Now().UTC()) || access.ExpiresAt.After(time.Now().UTC().Add(7*24*time.Hour)) {
		return ErrAccess
	}
	r, err := s.read(ctx, tx, id, "FOR SHARE OF i")
	if err != nil || !r.Available || r.OwnerID != owner || r.ManifestHash != access.ManifestHash || r.Generation != access.Generation {
		return ErrAccess
	}
	if selectedTools(r.Package.Manifest, access.Tools) != nil || validateScopes(resources(r), access.Resources) != nil {
		return ErrAccess
	}
	for _, tool := range access.Tools {
		if !has(r.AllowedTools, tool) {
			return ErrAccess
		}
	}
	for _, ref := range access.CredentialRefs {
		found := false
		for _, installed := range r.CredentialRefs {
			if installed == ref {
				found = true
			}
		}
		if !found {
			return ErrAccess
		}
	}
	var storedRevision int64
	var currentRevision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM extension_task_access WHERE task_id=$1 AND extension_id=$2 FOR UPDATE`, access.TaskID, id).Scan(&currentRevision)
	if err == pgx.ErrNoRows {
		if access.Revision != 0 {
			return ErrConflict
		}
	} else if err != nil || currentRevision != access.Revision {
		return ErrConflict
	}
	err = tx.QueryRow(ctx, `INSERT INTO extension_task_access(task_id,extension_id,owner_id,task_revision,constraints_revision,manifest_hash,generation,revision,tools_json,resources_json,credential_refs_json,expires_at,revoked,source_json) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12,$13::jsonb) ON CONFLICT(task_id,extension_id) DO UPDATE SET task_revision=EXCLUDED.task_revision,constraints_revision=EXCLUDED.constraints_revision,manifest_hash=EXCLUDED.manifest_hash,generation=EXCLUDED.generation,revision=extension_task_access.revision+1,tools_json=EXCLUDED.tools_json,resources_json=EXCLUDED.resources_json,credential_refs_json=EXCLUDED.credential_refs_json,expires_at=EXCLUDED.expires_at,revoked=EXCLUDED.revoked,source_json=EXCLUDED.source_json WHERE extension_task_access.owner_id=EXCLUDED.owner_id AND extension_task_access.revision=$14 RETURNING revision`, access.TaskID, id, owner, revision, constraints, access.ManifestHash, access.Generation, encoded(access.Tools), encoded(access.Resources), encoded(access.CredentialRefs), access.ExpiresAt, access.Revoked, encoded(source), access.Revision).Scan(&storedRevision)
	if err != nil || access.Revision == 0 && storedRevision != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}
func (s *Store) ReadAccess(ctx context.Context, q querier, task, id string, lock bool) (TaskAccess, error) {
	var a TaskAccess
	var tools, scopes, refs string
	suffix := ""
	if lock {
		suffix = " FOR SHARE"
	}
	err := q.QueryRow(ctx, `SELECT task_id,task_revision,constraints_revision,manifest_hash,generation,revision,tools_json::text,resources_json::text,credential_refs_json::text,expires_at,revoked FROM extension_task_access WHERE task_id=$1 AND extension_id=$2 AND owner_id=$3`+suffix, task, id, s.ownerID).Scan(&a.TaskID, &a.TaskRevision, &a.ConstraintsRevision, &a.ManifestHash, &a.Generation, &a.Revision, &tools, &scopes, &refs, &a.ExpiresAt, &a.Revoked)
	if err != nil || json.Unmarshal([]byte(tools), &a.Tools) != nil || json.Unmarshal([]byte(scopes), &a.Resources) != nil || json.Unmarshal([]byte(refs), &a.CredentialRefs) != nil {
		return TaskAccess{}, ErrAccess
	}
	return a, nil
}
func (s *Store) TaskAccess(ctx context.Context, task, id string) (TaskAccess, error) {
	if _, err := s.actor(ctx); err != nil {
		return TaskAccess{}, err
	}
	return s.ReadAccess(ctx, s.pool, task, id, false)
}

func (s *Store) Versions(ctx context.Context, id string) ([]Package, error) {
	if _, err := s.actor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT manifest_text FROM extension_versions WHERE extension_id=$1 ORDER BY installed_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Package{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		p, e := Parse(strings.NewReader(raw))
		if e != nil {
			return nil, e
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
