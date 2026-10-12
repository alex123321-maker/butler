package task

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/jackc/pgx/v5"
)

type AuthorityOverview struct {
	Available       bool   `json:"available"`
	GoalRevision    int64  `json:"goal_revision"`
	GoalStatus      string `json:"goal_status"`
	MandateRevision int64  `json:"mandate_revision"`
	MandateStatus   string `json:"mandate_status"`
	VersionsCurrent bool   `json:"versions_current"`
}

type AttemptOverview struct {
	RunID           string `json:"run_id"`
	TaskRevision    int64  `json:"task_revision"`
	State           string `json:"state"`
	AnswerState     string `json:"answer_state"`
	DeliveryState   string `json:"delivery_state"`
	ResultID        string `json:"result_id,omitempty"`
	ArtifactID      string `json:"artifact_id,omitempty"`
	PrivacyRedacted bool   `json:"privacy_redacted"`
}

type EvidenceReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type ResultOverview struct {
	ResultID          string              `json:"result_id"`
	TaskRevision      int64               `json:"task_revision"`
	CurrentRevision   bool                `json:"current_revision"`
	RunID             string              `json:"run_id,omitempty"`
	Verdict           string              `json:"verdict"`
	Evidence          []EvidenceReference `json:"evidence_refs"`
	OmittedReferences int                 `json:"omitted_reference_count"`
}

var opaqueOwnerRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,255}$`)
var ownerRefHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func safeDataRefs(refs []actionauth.DataRef) []actionauth.DataRef {
	result := []actionauth.DataRef{}
	for _, ref := range refs {
		if opaqueOwnerRef.MatchString(ref.ID) && ref.Revision > 0 && ref.AccessRevision > 0 && ownerRefHash.MatchString(ref.Hash) {
			result = append(result, ref)
		}
	}
	return result
}

// References are finite typed identifiers. Arbitrary owner/model text, URLs,
// query strings and private host paths are never copied into this projection.
func safeEvidenceRefs(refs []string) ([]EvidenceReference, int) {
	result := []EvidenceReference{}
	for _, ref := range refs {
		kind, id, ok := strings.Cut(ref, ":")
		if ok && (kind == "source" || kind == "artifact" || kind == "effect" || kind == "result") && opaqueOwnerRef.MatchString(id) {
			result = append(result, EvidenceReference{Kind: kind, ID: id})
		}
	}
	return result, len(refs) - len(result)
}

// This is a content-free snapshot of stored versions, not an authorization
// verdict. Admission additionally checks current scope, preferences and sources.
func ownerAuthorityOverview(ctx context.Context, tx pgx.Tx, a actionauth.Assignment, at time.Time) (*AuthorityOverview, error) {
	v := &AuthorityOverview{}
	var goalExpiry, mandateExpiry time.Time
	var policyRevision, mandateGoalRevision int64
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT g.revision,g.status,g.expires_at,m.revision,m.status,m.expires_at,m.revoked_at,m.goal_revision,COALESCE(p.revision,0) FROM owner_goals g JOIN action_mandates m ON m.goal_id=g.goal_id AND m.scope_kind='owner_goal' AND m.owner_id=g.owner_id LEFT JOIN owner_purpose_policies p ON p.owner_id=g.owner_id AND p.purpose=$4 WHERE g.goal_id=$1 AND m.mandate_id=$2 AND g.owner_id=$3`, a.GoalID, a.MandateID, a.OwnerID, a.Purpose).Scan(&v.GoalRevision, &v.GoalStatus, &goalExpiry, &v.MandateRevision, &v.MandateStatus, &mandateExpiry, &revoked, &mandateGoalRevision, &policyRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	v.Available = true
	v.VersionsCurrent = v.GoalRevision == a.GoalRevision && v.MandateRevision == a.MandateRevision && mandateGoalRevision == a.GoalRevision && policyRevision == a.PolicyRevision && v.GoalStatus == "active" && v.MandateStatus == "active" && revoked == nil && goalExpiry.After(at) && mandateExpiry.After(at)
	return v, nil
}

func ownerEvidence(ctx context.Context, tx pgx.Tx, id string, revision int64) ([]AttemptOverview, bool, []ResultOverview, bool, error) {
	attempts, results := []AttemptOverview{}, []ResultOverview{}
	rows, err := tx.Query(ctx, `SELECT r.run_id,COALESCE(r.task_revision,0),r.current_state,COALESCE(f.answer_state,'unavailable'),COALESCE(d.status,'unavailable'),COALESCE(f.result_id,''),COALESCE(f.artifact_id,''),COALESCE((to_jsonb(f)->>'privacy_redacted')::boolean,false) FROM runs r LEFT JOIN run_final_results f USING(run_id) LEFT JOIN LATERAL(SELECT CASE WHEN count(*)=0 THEN 'unavailable' WHEN bool_or(status='delivery_unknown') THEN 'delivery_unknown' WHEN bool_or(status='failed') THEN 'failed' WHEN bool_or(status='sending') THEN 'sending' WHEN bool_or(status='pending') THEN 'pending' WHEN bool_and(status='result_ready') THEN 'result_ready' WHEN bool_and(status='sent') THEN 'sent' ELSE 'partial' END status FROM delivery_outbox WHERE run_id=r.run_id) d ON TRUE WHERE r.task_id=$1 ORDER BY r.started_at DESC,r.run_id LIMIT 21`, id)
	if err != nil {
		return nil, false, nil, false, err
	}
	for rows.Next() {
		var v AttemptOverview
		if err = rows.Scan(&v.RunID, &v.TaskRevision, &v.State, &v.AnswerState, &v.DeliveryState, &v.ResultID, &v.ArtifactID, &v.PrivacyRedacted); err != nil {
			rows.Close()
			return nil, false, nil, false, err
		}
		attempts = append(attempts, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, nil, false, err
	}
	rows, err = tx.Query(ctx, `SELECT result_id,task_revision,COALESCE(run_id,''),verdict,evidence_refs_json FROM task_results WHERE task_id=$1 ORDER BY created_at DESC,result_id LIMIT 21`, id)
	if err != nil {
		return nil, false, nil, false, err
	}
	for rows.Next() {
		var v ResultOverview
		var raw []byte
		var refs []string
		if err = rows.Scan(&v.ResultID, &v.TaskRevision, &v.RunID, &v.Verdict, &raw); err != nil {
			rows.Close()
			return nil, false, nil, false, err
		}
		if json.Unmarshal(raw, &refs) != nil {
			rows.Close()
			return nil, false, nil, false, ErrInvalid
		}
		v.CurrentRevision = v.TaskRevision == revision
		v.Evidence, v.OmittedReferences = safeEvidenceRefs(refs)
		results = append(results, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, nil, false, err
	}
	truncatedAttempts, truncatedResults := len(attempts) > 20, len(results) > 20
	if truncatedAttempts {
		attempts = attempts[:20]
	}
	if truncatedResults {
		results = results[:20]
	}
	return attempts, truncatedAttempts, results, truncatedResults, nil
}
