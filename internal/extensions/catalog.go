package extensions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func (s *Store) contract(r Record, op Operation) (*toolbrokerv1.ToolContract, error) {
	if !r.Available || !has(r.AllowedTools, op.Name) {
		return nil, ErrUnavailable
	}
	profile, err := s.profile(r.Package)
	if err != nil {
		return nil, err
	}
	e := &toolbrokerv1.ExtensionContract{ExtensionId: r.ID, ManifestHash: r.ManifestHash, Version: r.Package.Manifest.Version, Source: r.Package.Manifest.Source.URI, Generation: r.Generation, ProfilePin: hashJSON(profile), Method: op.Method, Effect: op.Effect, TimeoutMs: r.Package.Manifest.Limits.TimeoutMillis, EgressBytes: r.Package.Manifest.Limits.EgressBytes, OutputBytes: r.Package.Manifest.Limits.OutputBytes}
	for _, resource := range resources(r) {
		e.Resources = append(e.Resources, &toolbrokerv1.ExtensionResource{Origin: resource.Origin, PathPrefix: resource.PathPrefix})
	}
	for _, input := range r.Package.Manifest.Credentials {
		e.Credentials = append(e.Credentials, &toolbrokerv1.ExtensionCredential{Input: input.Input, Field: input.Field, Required: input.Required})
	}
	effect := &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}
	risk := "high"
	if op.Effect == "read" {
		effect.RetrySafety, effect.VerificationMethod, risk = "read_only", "read_result", "low"
	} else if receipt := op.Receipt; receipt != nil {
		origin := r.Configuration[r.Package.Manifest.Resources[0].OriginSetting]
		effect = &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: receipt.Protocol, IdempotencyHeader: receipt.IdempotencyHeader, ScopeOrigin: origin, VerificationUrl: origin + receipt.VerificationPath}
		e.Receipt = effect
	}
	return &toolbrokerv1.ToolContract{ToolName: op.Name, Description: op.Description, ToolClass: "extension", InputSchemaJson: string(op.InputSchema), OutputSchemaJson: string(op.OutputSchema), RuntimeTarget: profile.Target, RiskLevel: risk, SupportsCredentialRefs: len(e.Credentials) > 0, Status: "enabled", ContractRevision: fmt.Sprintf("%s:%d", r.ManifestHash, r.Generation), EffectContract: effect, Extension: e}, nil
}
func (s *Store) GetContract(ctx context.Context, name string) (*toolbrokerv1.ToolContract, error) {
	parts := strings.Split(name, ".")
	if len(parts) != 3 || parts[0] != "extension" {
		return nil, ErrUnavailable
	}
	r, err := s.Get(ctx, parts[1])
	if err != nil {
		return nil, err
	}
	op, err := operation(r.Package.Manifest, name)
	if err != nil {
		return nil, err
	}
	return s.contract(r, op)
}
func (s *Store) ProbeContract(r Record) (*toolbrokerv1.ExtensionContract, string, error) {
	profile, err := s.profile(r.Package)
	if err != nil {
		return nil, "", err
	}
	e := &toolbrokerv1.ExtensionContract{ExtensionId: r.ID, ManifestHash: r.ManifestHash, Version: r.Package.Manifest.Version, Source: r.Package.Manifest.Source.URI, Generation: r.Generation, ProfilePin: hashJSON(profile), Method: "GET", Effect: "read", TimeoutMs: r.Package.Manifest.Limits.TimeoutMillis, EgressBytes: r.Package.Manifest.Limits.EgressBytes, OutputBytes: r.Package.Manifest.Limits.OutputBytes}
	for _, resource := range resources(r) {
		e.Resources = append(e.Resources, &toolbrokerv1.ExtensionResource{Origin: resource.Origin, PathPrefix: resource.PathPrefix})
	}
	return e, profile.Target, nil
}

func (s *Store) accessForAction(ctx context.Context, tx pgx.Tx, r Record, b actionauth.ActionBinding, at time.Time) (int64, time.Time, error) {
	if !r.Available || r.OwnerID != b.OwnerID || b.OwnerID != s.ownerID || r.ManifestHash != b.ExtensionManifestHash || r.Generation != b.ExtensionGeneration || !has(r.AllowedTools, b.ToolName) || !resourceWithin(resources(r), b.Resource) {
		return 0, time.Time{}, ErrAccess
	}
	for _, ref := range b.CredentialRefs {
		found := false
		for _, installed := range r.CredentialRefs {
			if ref == installed {
				found = true
			}
		}
		if !found {
			return 0, time.Time{}, ErrAccess
		}
	}
	if b.GoalID != "" {
		a := actionauth.Assignment{TaskID: b.TaskID, OwnerID: b.OwnerID, GoalID: b.GoalID, GoalRevision: b.GoalRevision, MandateID: b.MandateID, MandateRevision: b.MandateRevision, Purpose: b.Purpose, PolicyRevision: b.OwnerPolicyRevision}
		m, _, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
		if err != nil || actionauth.CheckExtensionAttachment(m, r.Package.Manifest.Source.URI, r.ID, r.ManifestHash, []string{b.ToolName}, at) != nil || actionauth.CheckAssignedAction(m, b, at) != nil {
			return 0, time.Time{}, ErrAccess
		}
		return m.Revision, m.ExpiresAt, nil
	}
	a, err := s.ReadAccess(ctx, tx, b.TaskID, r.ID, true)
	if err != nil || a.Revoked || a.TaskRevision != b.TaskRevision || a.ConstraintsRevision != b.ConstraintsRevision || a.ManifestHash != r.ManifestHash || a.Generation != r.Generation || !a.ExpiresAt.After(at) || !has(a.Tools, b.ToolName) || !resourceWithin(a.Resources, b.Resource) {
		return 0, time.Time{}, ErrAccess
	}
	for _, ref := range b.CredentialRefs {
		found := false
		for _, allowed := range a.CredentialRefs {
			if ref == allowed {
				found = true
			}
		}
		if !found {
			return 0, time.Time{}, ErrAccess
		}
	}
	return a.Revision, a.ExpiresAt, nil
}
func (s *Store) BindAction(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, b actionauth.ActionBinding, c actionauth.ContextSnapshot) (actionauth.ActionBinding, error) {
	if b.ExtensionID == "" || b.TaskID == "" || b.TaskRevision <= 0 || b.ConstraintsRevision <= 0 || c.TaskID != b.TaskID || c.TaskStatus != "active" || contract.GetExtension() == nil {
		return b, ErrAccess
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return b, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return b, err
	}
	var status, owner string
	var revision, constraints int64
	if err = tx.QueryRow(ctx, `SELECT status,owner_id,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, b.TaskID).Scan(&status, &owner, &revision, &constraints); err != nil || status != "active" || owner != b.OwnerID || revision != b.TaskRevision || constraints != b.ConstraintsRevision {
		return b, ErrAccess
	}
	// No new task authority is inferred from a package or its descriptions.
	if err = actionauth.CheckTaskActionTx(ctx, tx, b, at); err != nil {
		return b, err
	}
	r, err := s.read(ctx, tx, b.ExtensionID, "FOR SHARE OF i")
	if err != nil {
		return b, err
	}
	op, err := operation(r.Package.Manifest, b.ToolName)
	if err != nil {
		return b, err
	}
	current, err := s.contract(r, op)
	if err != nil {
		return b, err
	}
	hash, err := actionauth.ContractHash(current)
	if err != nil || hash != b.ContractHash {
		return b, ErrConflict
	}
	if err = checkInputs(call, r, op); err != nil {
		return b, err
	}
	revision, expiry, err := s.accessForAction(ctx, tx, r, b, at)
	if err != nil {
		return b, err
	}
	if r.HealthyUntil.Before(expiry) {
		expiry = *r.HealthyUntil
	}
	return actionauth.BindExtensionAccess(b, revision, expiry), tx.Commit(ctx)
}
func checkInputs(call *toolbrokerv1.ToolCall, r Record, op Operation) error {
	var args map[string]any
	if DecodeArguments(call.GetArgsJson(), &args) != nil {
		return ErrAccess
	}
	for key := range args {
		if key != "url" && key != "body" && key != "authorization" {
			return ErrAccess
		}
	}
	refs := 0
	var count func(any)
	count = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if v["type"] == "credential_ref" {
				refs++
			}
			for _, child := range v {
				count(child)
			}
		case []any:
			for _, child := range v {
				count(child)
			}
		}
	}
	count(args)
	if refs != len(call.GetCredentialRefs()) || refs > 1 {
		return ErrAccess
	}
	if op.Method == "GET" || op.Method == "HEAD" {
		if _, hasBody := args["body"]; hasBody {
			return ErrAccess
		}
	}
	for _, input := range r.Package.Manifest.Credentials {
		value, exists := args[input.Input]
		if !exists {
			if input.Required {
				return ErrAccess
			}
			continue
		}
		object, ok := value.(map[string]any)
		if !ok || len(object) != 3 || object["type"] != "credential_ref" || object["field"] != input.Field {
			return ErrAccess
		}
		ref := r.CredentialRefs[input.Input]
		if ref.Alias == "" || object["alias"] != ref.Alias || object["field"] != ref.Field {
			return ErrAccess
		}
		if len(call.GetCredentialRefs()) != 1 || call.GetCredentialRefs()[0].GetType() != "credential_ref" || call.GetCredentialRefs()[0].GetAlias() != ref.Alias || call.GetCredentialRefs()[0].GetField() != ref.Field {
			return ErrAccess
		}
	}
	if _, hasAuth := args["authorization"]; hasAuth {
		found := false
		for _, input := range r.Package.Manifest.Credentials {
			if input.Input == "authorization" {
				found = true
			}
		}
		if !found {
			return ErrAccess
		}
	} else if refs != 0 {
		return ErrAccess
	}
	return nil
}
func (s *Store) CheckActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	if b.ExtensionID == "" {
		return nil
	}
	if b.TaskID == "" || b.ExtensionAccessRevision <= 0 || b.ExtensionAccessExpiresAt == nil {
		return ErrAccess
	}
	var at time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return err
	}
	r, err := s.read(ctx, tx, b.ExtensionID, "FOR SHARE OF i")
	if err != nil {
		return err
	}
	op, err := operation(r.Package.Manifest, b.ToolName)
	if err != nil {
		return err
	}
	current, err := s.contract(r, op)
	if err != nil {
		return err
	}
	hash, err := actionauth.ContractHash(current)
	if err != nil || hash != b.ContractHash {
		return ErrConflict
	}
	revision, expiry, err := s.accessForAction(ctx, tx, r, b, at)
	if err != nil {
		return err
	}
	if revision != b.ExtensionAccessRevision || !b.ExtensionAccessExpiresAt.After(at) || b.ExtensionAccessExpiresAt.After(expiry) || r.HealthyUntil == nil || b.ExtensionAccessExpiresAt.After(*r.HealthyUntil) {
		return ErrAccess
	}
	return nil
}

// Catalog first filters current installation/task/owner authority, then performs
// deterministic search. A future ranker may reorder this set, never add IDs.
func (s *Store) Catalog(ctx context.Context, c actionauth.ContextSnapshot, query string, limit int) ([]*toolbrokerv1.ToolContract, error) {
	if c.OwnerID != s.ownerID || c.TaskID == "" {
		return nil, nil
	}
	if c.PinnedTaskRevision != c.TaskRevision || c.TaskStatus != "active" || c.LeaseID == "" || c.LeaseEpoch <= 0 || !c.LeaseExpiresAt.After(time.Now().UTC()) || len(query) > 256 {
		return nil, ErrAccess
	}
	if limit <= 0 {
		limit = 32
	}
	if limit > 64 {
		limit = 64
	}
	// Close the list cursor before acquiring another connection (MaxConns=2).
	rows, err := s.pool.Query(ctx, `SELECT extension_id FROM extension_installations WHERE owner_id=$1 AND status='enabled' ORDER BY extension_id`, s.ownerID)
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
	type candidate struct {
		contract *toolbrokerv1.ToolContract
		score    int
	}
	candidates := []candidate{}
	terms := strings.Fields(strings.ToLower(query))
	for _, id := range ids {
		r, err := s.Get(ctx, id)
		if err != nil || !r.Available {
			continue
		}
		for _, op := range r.Package.Manifest.Operations {
			contract, err := s.contract(r, op)
			if err != nil {
				continue
			}
			effect := actionauth.Effect(op.Effect)
			if actionauth.CheckExecutionPolicy(c, effect) != nil || c.AutonomyMode == 1 && (effect != actionauth.EffectRead || len(r.Package.Manifest.Credentials) > 0) {
				continue
			}
			allowed := false
			candidateScopes := resources(r)
			if c.Authority != nil {
				if m, e := actionauth.NewPostgresStore(s.pool).GetMandate(ctx, c.Authority.MandateID); e == nil {
					candidateScopes = append(candidateScopes, m.ResourceScopes...)
				}
			} else if access, e := s.ReadAccess(ctx, s.pool, c.TaskID, r.ID, false); e == nil {
				candidateScopes = append(candidateScopes, access.Resources...)
			}
			for _, resource := range candidateScopes {
				b := actionauth.ActionBinding{TaskID: c.TaskID, OwnerID: c.OwnerID, TaskRevision: c.TaskRevision, ConstraintsRevision: c.ConstraintsRevision, ToolName: op.Name, Resource: resource.Origin + resource.PathPrefix, Effect: effect, ExtensionID: r.ID, ExtensionManifestHash: r.ManifestHash, ExtensionGeneration: r.Generation}
				if a := c.Authority; a != nil {
					b.GoalID, b.GoalRevision, b.MandateID, b.MandateRevision, b.Purpose, b.OwnerPolicyRevision = a.GoalID, a.GoalRevision, a.MandateID, a.MandateRevision, a.Purpose, a.PolicyRevision
				}
				tx, e := s.pool.Begin(ctx)
				if e != nil {
					return nil, e
				}
				at := time.Now().UTC()
				e = actionauth.CheckTaskActionTx(ctx, tx, b, at)
				if e == nil {
					_, _, e = s.accessForAction(ctx, tx, r, b, at)
				}
				_ = tx.Rollback(ctx)
				if e == nil {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
			score := 0
			haystack := strings.ToLower(op.Name + " " + op.Description + " " + strings.Join(op.Tags, " "))
			for _, term := range terms {
				if strings.Contains(haystack, term) {
					score++
				}
			}
			if len(terms) > 0 && score != len(terms) {
				continue
			}
			candidates = append(candidates, candidate{contract, score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].contract.GetToolName() < candidates[j].contract.GetToolName()
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	result := make([]*toolbrokerv1.ToolContract, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, item.contract)
	}
	return result, nil
}
