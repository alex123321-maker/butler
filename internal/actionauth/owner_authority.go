package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/butler/butler/internal/delegation"
	"github.com/jackc/pgx/v5"
)

var ErrOwnerAuthority = errors.New("owner goal or mandate authority unavailable, stale or outside scope")

func GetTaskAssignment(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, taskID string) (Assignment, error) {
	var a Assignment
	err := q.QueryRow(ctx, `SELECT task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref,policy_revision FROM task_authority_bindings WHERE task_id=$1`, taskID).Scan(&a.TaskID, &a.OwnerID, &a.GoalID, &a.GoalRevision, &a.MandateID, &a.MandateRevision, &a.Purpose, &a.InitiativeReason, &a.ObservationRef, &a.PolicyRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, nil
	}
	return a, err
}

// CheckTaskAuthorityTx is called after current task/run locks. A confirmed
// authority mutation commits without waiting on worker ownership. Shared locks
// remain until the caller commits its dispatch, result or admission transaction.
// No binding preserves ordinary task/conversation behavior.
func CheckTaskAuthorityTx(ctx context.Context, tx pgx.Tx, taskID, ownerID string, expected *Assignment, at time.Time) (Assignment, error) {
	if err := delegation.CheckTaskTx(ctx, tx, taskID); err != nil {
		return Assignment{}, err
	}
	a, err := GetTaskAssignment(ctx, tx, taskID)
	if err != nil {
		return a, err
	}
	if a.TaskID == "" {
		if expected != nil && expected.GoalID != "" {
			return a, ErrOwnerAuthority
		}
		return a, nil
	}
	if a.OwnerID != ownerID || expected != nil && (expected.GoalID != a.GoalID || expected.GoalRevision != a.GoalRevision || expected.MandateID != a.MandateID || expected.MandateRevision != a.MandateRevision || expected.Purpose != a.Purpose || expected.PolicyRevision != a.PolicyRevision) {
		return a, ErrOwnerAuthority
	}
	_, _, err = CheckAssignmentTx(ctx, tx, a, at)
	return a, err
}

// CheckAssignmentTx never locks an existing task. Its caller must acquire all
// ownership/session/run/task locks first. The order here is mandate then goal.
func CheckAssignmentTx(ctx context.Context, tx pgx.Tx, a Assignment, at time.Time) (Mandate, GoalPolicy, error) {
	var m Mandate
	var g GoalPolicy
	var data []byte
	var status, scope, owner, goalID string
	var rev, goalRev int64
	var expiry time.Time
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT owner_id,revision,mandate_data,status,scope_kind,COALESCE(goal_id,''),goal_revision,expires_at,revoked_at FROM action_mandates WHERE mandate_id=$1 FOR SHARE`, a.MandateID).Scan(&owner, &rev, &data, &status, &scope, &goalID, &goalRev, &expiry, &revoked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return m, g, ErrOwnerAuthority
		}
		return m, g, err
	}
	if json.Unmarshal(data, &m) != nil {
		return m, g, ErrOwnerAuthority
	}
	m.Revision, m.Status, m.ScopeKind, m.ExpiresAt, m.RevokedAt = rev, status, scope, expiry, revoked
	if owner != a.OwnerID || m.OwnerID != owner || scope != "owner_goal" || rev != a.MandateRevision || goalID != a.GoalID || goalRev != a.GoalRevision || status != "active" || revoked != nil || !expiry.After(at) || m.Purpose != a.Purpose || !WithinActiveHours(m.Hours, at) {
		return m, g, ErrOwnerAuthority
	}
	var kind string
	var goalStatus string
	var currentRevision int64
	var goalExpiry time.Time
	err = tx.QueryRow(ctx, `SELECT owner_id,kind,status,revision,expires_at,goal_data FROM owner_goals WHERE goal_id=$1 FOR SHARE`, a.GoalID).Scan(&owner, &kind, &goalStatus, &currentRevision, &goalExpiry, &data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return m, g, ErrOwnerAuthority
		}
		return m, g, err
	}
	if json.Unmarshal(data, &g) != nil {
		return m, g, ErrOwnerAuthority
	}
	// Durable row columns, rather than JSON alone, are authoritative.
	g.ID = a.GoalID
	g.OwnerID = owner
	g.Kind = kind
	g.Status = goalStatus
	g.Revision = currentRevision
	g.ExpiresAt = goalExpiry
	if owner != a.OwnerID || g.Kind != "objective" || g.Status != "active" || g.Revision != a.GoalRevision || !g.ExpiresAt.After(at) || g.Purpose != a.Purpose {
		return m, g, ErrOwnerAuthority
	}
	var policyRevision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM owner_purpose_policies WHERE owner_id=$1 AND purpose=$2 FOR SHARE`, a.OwnerID, a.Purpose).Scan(&policyRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return m, g, ErrOwnerAuthority
		}
		return m, g, err
	}
	if policyRevision != a.PolicyRevision {
		return m, g, ErrOwnerAuthority
	}
	rows, err := tx.Query(ctx, `SELECT goal_id,kind,revision,expires_at,goal_data FROM owner_goals WHERE owner_id=$1 AND goal_data->>'purpose'=$2 AND kind IN ('preference','constraint') AND status='active' ORDER BY goal_id`, a.OwnerID, a.Purpose)
	if err != nil {
		return m, g, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		var policy GoalPolicy
		var id, kind string
		var revision int64
		var expires time.Time
		if err = rows.Scan(&id, &kind, &revision, &expires, &data); err != nil {
			return m, g, err
		}
		if json.Unmarshal(data, &policy) != nil || !expires.After(at) {
			return m, g, ErrOwnerAuthority
		}
		policy.ID, policy.Kind, policy.Revision, policy.ExpiresAt = id, kind, revision, expires
		policy.OwnerID, policy.Status = a.OwnerID, "active"
		policy.Policies = nil
		g.Policies = append(g.Policies, policy)
	}
	if err = rows.Err(); err != nil {
		return m, g, err
	}
	return m, g, nil
}

func WithinActiveHours(h *ActiveHours, at time.Time) bool {
	if h == nil {
		return true
	}
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		return false
	}
	return withinHours(h, loc, at)
}

func withinHours(h *ActiveHours, loc *time.Location, at time.Time) bool {
	local := at.In(loc)
	minute := local.Hour()*60 + local.Minute()
	dayOK := false
	for _, d := range h.Weekdays {
		if d == int(local.Weekday()) {
			dayOK = true
		}
	}
	if !dayOK {
		return false
	}
	if h.StartMinute < h.EndMinute {
		return minute >= h.StartMinute && minute < h.EndMinute
	}
	if h.StartMinute > h.EndMinute {
		return minute >= h.StartMinute || minute < h.EndMinute
	}
	return false
}

func validResourceScope(s ResourceScope) bool {
	if strings.HasPrefix(s.Origin, "tool:") {
		name := strings.TrimPrefix(s.Origin, "tool:")
		return s.PathPrefix == "" && name != "" && strings.Contains(name, ".") && !strings.ContainsAny(name, "*?/\\:# \t\r\n")
	}
	u, err := url.Parse(s.Origin)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, "*? ") {
		return false
	}
	return strings.HasPrefix(s.PathPrefix, "/") && path.Clean(s.PathPrefix) == strings.TrimSuffix(s.PathPrefix, "/") && !strings.ContainsAny(s.PathPrefix, "%?\\#") || s.PathPrefix == "/"
}

func resourceWithin(scope ResourceScope, raw string) bool {
	if strings.HasPrefix(scope.Origin, "tool:") {
		return validResourceScope(scope) && raw == scope.Origin
	}
	if strings.HasPrefix(raw, "browser:") || strings.HasPrefix(raw, "single_tab:") {
		if i := strings.Index(raw, "http"); i >= 0 {
			raw = raw[i:]
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || strings.ToLower(u.Scheme+"://"+u.Host) != strings.ToLower(scope.Origin) {
		return false
	}
	// Encoded separators/traversal and dot segments are ambiguous between remote
	// servers. Refuse them rather than widening an account/path permission.
	p := u.Path
	if p == "" {
		p = "/"
	}
	escaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%2e") || strings.Contains(p, "\\") || path.Clean(p) != strings.TrimSuffix(p, "/") && p != "/" {
		return false
	}
	prefix := strings.TrimSuffix(scope.PathPrefix, "/")
	return prefix == "" || p == prefix || strings.HasPrefix(p, prefix+"/")
}

func ValidateOwnerMandate(m Mandate, now time.Time) error {
	if m.ID == "" || m.OwnerID == "" || m.ScopeKind != "owner_goal" || m.GoalID == "" || m.GoalRevision <= 0 || m.Revision <= 0 || m.TaskID != "" || m.Purpose == "" {
		return ErrOwnerAuthority
	}
	if m.Status != "active" && m.Status != "paused" && m.Status != "revoked" {
		return ErrOwnerAuthority
	}
	if m.Status == "active" && !m.ExpiresAt.After(now) || m.ExpiresAt.After(now.Add(366*24*time.Hour)) || m.BudgetDeadline.IsZero() || m.Revision == 1 && m.BudgetDeadline.After(m.ExpiresAt) {
		return fmt.Errorf("mandate requires a finite deadline within one year and immutable budget deadline")
	}
	// Owner revisions may shorten current authority without changing the original
	// quota period. The goal service pins that period and all limits to the old row.
	if m.Privacy != "local_only" && m.Privacy != "cloud_allowed" {
		return fmt.Errorf("explicit model privacy policy required")
	}
	if len(m.Tools) == 0 || len(m.Tools) > 64 || len(m.ResourceScopes) == 0 || len(m.ResourceScopes) > 64 || len(m.Effects) == 0 || len(m.Intervention) == 0 || m.Reporting == "" {
		return fmt.Errorf("purpose, bounded capabilities/resources, intervention and reporting required")
	}
	if m.Reporting != "exceptions" && m.Reporting != "daily" && m.Reporting != "each_result" {
		return fmt.Errorf("explicit reporting policy required")
	}
	for _, boundary := range m.Intervention {
		switch boundary {
		case "effect_unknown", "scope_change", "budget_exhausted", "credential_required":
		default:
			return fmt.Errorf("unsupported intervention boundary")
		}
	}
	for _, name := range append(append([]string{}, m.Tools...), m.DenyTools...) {
		if name == "" || strings.ContainsAny(name, "*? ") || !strings.Contains(name, ".") {
			return fmt.Errorf("exact tool names required")
		}
	}
	for _, scope := range append(append([]ResourceScope{}, m.ResourceScopes...), m.DenyResources...) {
		if !validResourceScope(scope) {
			return fmt.Errorf("exact origin and canonical path prefix required")
		}
	}
	for _, e := range m.Effects {
		if e != EffectRead && e != EffectPrepare && e != EffectCommit && e != EffectUnknown {
			return ErrOwnerAuthority
		}
	}
	for _, r := range m.CredentialRefs {
		if r.Alias == "" || r.Field == "" || strings.ContainsAny(r.Alias+r.Field, "*?") {
			return fmt.Errorf("exact credential metadata references required")
		}
	}
	for _, n := range []int64{m.Budget.ModelTurns, m.Budget.Tokens, m.Budget.CostMicros, m.Budget.ExternalCalls, m.Budget.Children, m.Budget.ActiveMillis, m.Budget.EgressBytes, m.Budget.ProviderInputBytes} {
		if n < 0 || n > 1<<50 {
			return fmt.Errorf("finite nonnegative budget required")
		}
	}
	if m.Budget.ModelTurns == 0 || m.Budget.Tokens == 0 || m.Budget.ActiveMillis == 0 {
		return fmt.Errorf("positive model, token and duration limits required")
	}
	if m.Hours != nil {
		h := m.Hours
		if _, err := time.LoadLocation(h.Timezone); err != nil {
			return err
		}
		if h.StartMinute < 0 || h.StartMinute >= 1440 || h.EndMinute < 0 || h.EndMinute > 1440 || h.StartMinute == h.EndMinute || len(h.Weekdays) == 0 {
			return fmt.Errorf("bounded active hours required")
		}
		for _, d := range h.Weekdays {
			if d < 0 || d > 6 {
				return ErrOwnerAuthority
			}
		}
	}
	for _, e := range m.Extensions {
		if e.Source == "" || e.ExtensionID == "" || len(e.ManifestHash) != 64 || len(e.Tools) == 0 {
			return fmt.Errorf("exact extension source, identity, manifest digest and capabilities required")
		}
		for _, tool := range e.Tools {
			if !contains(m.Tools, tool) {
				return fmt.Errorf("extension capabilities exceed mandate")
			}
		}
	}
	if len(m.Converters) > 8 {
		return ErrOwnerAuthority
	}
	seenConverters := map[string]bool{}
	for _, p := range m.Converters {
		if p.Validate() != nil || seenConverters[p.ConverterID] || !contains(m.Tools, "converter."+p.ConverterID+".convert") {
			return ErrOwnerAuthority
		}
		seenConverters[p.ConverterID] = true
	}
	return nil
}

func ownerMandateAllows(m Mandate, b ActionBinding, at time.Time) bool {
	if m.ScopeKind != "owner_goal" || m.Status != "active" || m.RevokedAt != nil || !m.ExpiresAt.After(at) || !WithinActiveHours(m.Hours, at) || m.OwnerID != b.OwnerID || m.GoalID != b.GoalID || m.GoalRevision != b.GoalRevision || m.ID != b.MandateID || m.Revision != b.MandateRevision || m.Purpose != b.Purpose || contains(m.DenyTools, b.ToolName) || !contains(m.Tools, b.ToolName) {
		return false
	}
	for _, s := range m.DenyResources {
		if resourceWithin(s, b.Resource) {
			return false
		}
	}
	resourceOK := false
	for _, s := range m.ResourceScopes {
		if resourceWithin(s, b.Resource) {
			resourceOK = true
		}
	}
	effectOK := false
	for _, e := range m.Effects {
		if e == b.Effect {
			effectOK = true
		}
	}
	if !resourceOK || !effectOK {
		return false
	}
	for _, r := range b.CredentialRefs {
		found := false
		for _, allowed := range m.CredentialRefs {
			if r == allowed {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func CheckAssignedAction(m Mandate, b ActionBinding, at time.Time) error {
	if !ownerMandateAllows(m, b, at) {
		return ErrOwnerAuthority
	}
	return nil
}

func ReadGoalDenialsTx(ctx context.Context, tx pgx.Tx, goalID, ownerID string, at time.Time) ([]string, []ResourceScope, error) {
	rows, err := tx.Query(ctx, `SELECT mandate_data FROM action_mandates WHERE goal_id=$1 AND owner_id=$2 AND scope_kind='owner_goal' AND status='active' AND revoked_at IS NULL AND expires_at>$3 ORDER BY mandate_id`, goalID, ownerID, at)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var tools []string
	var resources []ResourceScope
	for rows.Next() {
		var data []byte
		var m Mandate
		if err = rows.Scan(&data); err != nil {
			return nil, nil, err
		}
		if json.Unmarshal(data, &m) != nil {
			return nil, nil, ErrOwnerAuthority
		}
		tools = append(tools, m.DenyTools...)
		resources = append(resources, m.DenyResources...)
	}
	return tools, resources, rows.Err()
}

func CheckDenyOverrides(b ActionBinding, tools []string, resources []ResourceScope) error {
	if contains(tools, b.ToolName) {
		return ErrOwnerAuthority
	}
	for _, scope := range resources {
		if resourceWithin(scope, b.Resource) {
			return ErrOwnerAuthority
		}
	}
	return nil
}

// CheckTaskActionTx includes deny overrides from every current active mandate
// for this exact goal. The already shared-locked goal serializes mutations of
// this set; no semantic similarity or model-supplied mandate selection occurs.
func CheckTaskActionTx(ctx context.Context, tx pgx.Tx, b ActionBinding, at time.Time) error {
	d, err := delegation.LoadTx(ctx, tx, b.TaskID)
	if err != nil {
		return err
	}
	if !delegationAllows(d, b) {
		return delegation.ErrDenied
	}
	a, err := CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, &Assignment{GoalID: b.GoalID, GoalRevision: b.GoalRevision, MandateID: b.MandateID, MandateRevision: b.MandateRevision, Purpose: b.Purpose, PolicyRevision: b.OwnerPolicyRevision}, at)
	if err != nil || a.TaskID == "" {
		return err
	}
	m, _, err := CheckAssignmentTx(ctx, tx, a, at)
	if err != nil {
		return err
	}
	if err = CheckAssignedAction(m, b, at); err != nil {
		return err
	}
	tools, resources, err := ReadGoalDenialsTx(ctx, tx, a.GoalID, a.OwnerID, at)
	if err != nil {
		return err
	}
	return CheckDenyOverrides(b, tools, resources)
}

func CheckExtensionAttachment(m Mandate, source, id, manifestHash string, tools []string, at time.Time) error {
	if m.ScopeKind != "owner_goal" || m.Status != "active" || m.RevokedAt != nil || !m.ExpiresAt.After(at) || !WithinActiveHours(m.Hours, at) {
		return ErrOwnerAuthority
	}
	for _, permission := range m.Extensions {
		if permission.Source == source && permission.ExtensionID == id && permission.ManifestHash == manifestHash {
			for _, tool := range tools {
				if !contains(permission.Tools, tool) || contains(m.DenyTools, tool) {
					return ErrOwnerAuthority
				}
			}
			return nil
		}
	}
	return ErrOwnerAuthority
}
