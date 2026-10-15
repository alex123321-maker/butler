package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

// ConsolidationHandler is the only code-owned consumer that can carry an
// explicit finite cross-session authority manifest. No other handler inherits it.
const ConsolidationHandler = "memory-consolidation-v1"

func contextManifestJSON(pins []ModelContextPin) []byte {
	if len(pins) == 0 {
		return []byte("[]")
	}
	return jsonBytes(pins)
}

type ModelContextRef struct {
	SessionKey    string `json:"session_key"`
	OriginalRunID string `json:"original_run_id"`
	TaskID        string `json:"task_id"`
}

type ModelContextPin struct {
	ModelContextRef
	OwnerID             string                `json:"owner_id"`
	TaskRevision        int64                 `json:"task_revision"`
	ConstraintsRevision int64                 `json:"constraints_revision"`
	Authority           actionauth.Assignment `json:"authority"`
	Privacy             string                `json:"privacy"`
	QuietSeconds        int                   `json:"quiet_seconds"`
}

func orderedContextRefs(refs []ModelContextRef) ([]ModelContextRef, error) {
	if len(refs) > 8 {
		return nil, ErrClaim
	}
	refs = append([]ModelContextRef(nil), refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].SessionKey < refs[j].SessionKey })
	for i, r := range refs {
		if r.SessionKey == "" || len(r.SessionKey) > 512 || r.OriginalRunID == "" || len(r.OriginalRunID) > 512 || len(r.TaskID) > 512 || i > 0 && refs[i-1].SessionKey == r.SessionKey {
			return nil, ErrClaim
		}
	}
	return refs, nil
}

func validContextShape(p ModelPolicy) error {
	if len(p.Contexts) == 0 && len(p.ContextPins) == 0 {
		return nil
	}
	if p.Handler != ConsolidationHandler || p.Job.TaskID != "" || p.Job.OriginalRunID != "" || p.Privacy != "cloud_allowed" || len(p.Contexts) == 0 || len(p.Contexts) != len(p.ContextPins) {
		return ErrClaim
	}
	refs, err := orderedContextRefs(p.Contexts)
	if err != nil {
		return err
	}
	for i, r := range refs {
		pin := p.ContextPins[i]
		if pin.ModelContextRef != r || pin.OwnerID != p.OwnerID || pin.Privacy != p.Privacy || pin.QuietSeconds != p.IntervalSeconds {
			return ErrClaim
		}
		if pin.TaskID == "" && (pin.TaskRevision != 0 || pin.ConstraintsRevision != 0 || pin.Authority != (actionauth.Assignment{})) {
			return ErrClaim
		}
		if pin.TaskID != "" && (pin.TaskRevision < 1 || pin.ConstraintsRevision < 1) {
			return ErrClaim
		}
	}
	return nil
}

// modelFingerprint preserves exact legacy bytes for nil contexts. New model
// manifests bind immutable source identities AND every authority context pin.
func modelFingerprint(refs []catalog.SourceRef, pins []ModelContextPin) (string, error) {
	ordered, sum, err := canonical(refs)
	if err != nil || len(pins) == 0 {
		return sum, err
	}
	if len(pins) > 8 {
		return "", ErrClaim
	}
	for i, p := range pins {
		if p.SessionKey == "" || p.OriginalRunID == "" || p.OwnerID == "" || p.Privacy != "cloud_allowed" || p.QuietSeconds < 60 || p.QuietSeconds > 86400 || i > 0 && pins[i-1].SessionKey >= p.SessionKey {
			return "", ErrClaim
		}
	}
	raw := jsonBytes(struct {
		Sources  []catalog.SourceRef `json:"sources"`
		Contexts []ModelContextPin   `json:"contexts"`
	}{ordered, pins})
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:]), nil
}

// lockContextTasksTx acquires all task rows before any assignment/goal lock.
// Sessions must already be locked by lockPolicySessionsTx.
func lockContextTasksTx(ctx context.Context, tx pgx.Tx, refs []ModelContextRef) error {
	ids := []string{}
	for _, r := range refs {
		if r.TaskID != "" {
			ids = append(ids, r.TaskID)
		}
	}
	rows, err := tx.Query(ctx, `SELECT task_id FROM tasks WHERE task_id=ANY($1) ORDER BY task_id FOR SHARE`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

// resolveContextPinTx proves present owner/run/task/assignment/privacy and quiet
// eligibility. A completed run provides identity only, never a borrowed lease.
func resolveContextPinTx(ctx context.Context, tx pgx.Tx, owner string, ref ModelContextRef, quiet int) (ModelContextPin, error) {
	p := ModelContextPin{ModelContextRef: ref, OwnerID: owner, Privacy: "cloud_allowed", QuietSeconds: quiet}
	var permitted, quietEligible bool
	err := tx.QueryRow(ctx, `SELECT s.user_id=$3 AND COALESCE(r.task_id,'')=$4
 AND r.current_state IN ('completed','failed','cancelled','timed_out')
 AND NOT EXISTS(SELECT 1 FROM tasks t WHERE t.session_key=s.session_key AND t.task_id<>$4),
 NOT EXISTS(SELECT 1 FROM session_ownership o WHERE o.session_key=s.session_key AND o.expires_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM runs x WHERE x.session_key=s.session_key AND x.current_state NOT IN ('completed','failed','cancelled','timed_out'))
 AND NOT EXISTS(SELECT 1 FROM runs x WHERE x.session_key=s.session_key AND x.updated_at>clock_timestamp()-$5*INTERVAL '1 second')
 AND NOT EXISTS(SELECT 1 FROM messages m WHERE m.session_key=s.session_key AND m.created_at>clock_timestamp()-$5*INTERVAL '1 second')
 AND NOT EXISTS(SELECT 1 FROM tool_calls c JOIN runs x USING(run_id) WHERE x.session_key=s.session_key AND GREATEST(c.started_at,c.finished_at)>clock_timestamp()-$5*INTERVAL '1 second')
 FROM sessions s JOIN runs r USING(session_key) WHERE s.session_key=$1 AND r.run_id=$2 FOR SHARE OF r`, ref.SessionKey, ref.OriginalRunID, owner, ref.TaskID, quiet).Scan(&permitted, &quietEligible)
	if err != nil {
		if err == pgx.ErrNoRows {
			return p, ErrClaim
		}
		return p, err
	}
	if !permitted {
		return p, ErrClaim
	}
	if !quietEligible {
		return p, ErrWaiting
	}
	if ref.TaskID == "" {
		return p, nil
	}
	var actual, status, parent, session string
	err = tx.QueryRow(ctx, `SELECT owner_id,status,COALESCE(parent_task_id,''),session_key,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, ref.TaskID).Scan(&actual, &status, &parent, &session, &p.TaskRevision, &p.ConstraintsRevision)
	if err != nil {
		if err == pgx.ErrNoRows {
			return p, ErrClaim
		}
		return p, err
	}
	if actual != owner || session != ref.SessionKey || parent != "" || status != "active" && status != "completed" {
		return p, ErrClaim
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return p, err
	}
	p.Authority, err = actionauth.CheckTaskAuthorityTx(ctx, tx, ref.TaskID, owner, nil, now)
	if err != nil {
		return p, err
	}
	if p.Authority.TaskID != "" {
		m, _, e := actionauth.CheckAssignmentTx(ctx, tx, p.Authority, now)
		if e != nil {
			return p, e
		}
		if m.Privacy != "cloud_allowed" {
			return p, ErrClaim
		}
	}
	return p, nil
}

func checkContextPinsTx(ctx context.Context, tx pgx.Tx, owner string, pins []ModelContextPin) error {
	refs := make([]ModelContextRef, len(pins))
	for i, p := range pins {
		refs[i] = p.ModelContextRef
	}
	if err := lockContextTasksTx(ctx, tx, refs); err != nil {
		return err
	}
	for _, pin := range pins {
		current, err := resolveContextPinTx(ctx, tx, owner, pin.ModelContextRef, pin.QuietSeconds)
		if err != nil {
			return err
		}
		if current != pin {
			return ErrClaim
		}
	}
	return nil
}

func contextPermits(p ModelPolicy, src catalog.Source) bool {
	for _, pin := range p.ContextPins {
		if src.ScopeType == "session" && src.ScopeID == pin.SessionKey || src.ScopeType == "task" && pin.TaskID != "" && src.ScopeID == pin.TaskID {
			return true
		}
	}
	return false
}
