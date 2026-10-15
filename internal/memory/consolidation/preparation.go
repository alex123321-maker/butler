package consolidation

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

const MaxRetainedVersions = 8
const MaxRetainedOwner = 32

type Store struct {
	Catalog *catalog.Store
	Jobs    *jobs.Store
}

type PrepareRequest struct {
	ViewKey  string                  `json:"view_key"`
	Sources  []catalog.SourceRef     `json:"sources"`
	ClaimIDs []string                `json:"claim_ids"`
	Query    catalog.TemporalQuery   `json:"temporal_query"`
	Policy   jobs.ModelPolicyRequest `json:"policy"`
}

type inputRecord struct {
	SourceID, OwnerID, ViewKey, PolicyID, RequestHash string
	Sources                                           []catalog.SourceRef
	ClaimIDs                                          []string
	Query                                             catalog.TemporalQuery
	Retired, CleanupComplete                          bool
}

func readInput(ctx context.Context, tx pgx.Tx, policyID string) (inputRecord, error) {
	var in inputRecord
	var sources, claims, query []byte
	err := tx.QueryRow(ctx, `SELECT source_id,owner_id,view_key,policy_id,request_hash,source_units,claim_ids,temporal_query,retired,cleanup_complete FROM memory_consolidation_inputs WHERE policy_id=$1`, policyID).Scan(&in.SourceID, &in.OwnerID, &in.ViewKey, &in.PolicyID, &in.RequestHash, &sources, &claims, &query, &in.Retired, &in.CleanupComplete)
	if err != nil {
		return in, err
	}
	if json.Unmarshal(sources, &in.Sources) != nil || json.Unmarshal(claims, &in.ClaimIDs) != nil || json.Unmarshal(query, &in.Query) != nil {
		return in, catalog.ErrInvalid
	}
	return in, nil
}

func normalizePrepare(q PrepareRequest) (PrepareRequest, error) {
	if q.ViewKey == "" || len(q.ViewKey) > 128 || len(q.Policy.Contexts) < 1 || len(q.Policy.Contexts) > 8 || len(q.Sources)+len(q.ClaimIDs) < 1 || len(q.Sources)+len(q.ClaimIDs) > MaxUnits || q.Policy.MaxJobs != 1 || len(q.Policy.Job.Sources) != 0 || q.Policy.Job.TaskID != "" || q.Policy.Job.OriginalRunID != "" {
		return q, catalog.ErrInvalid
	}
	q.Policy.Handler = Version
	q.Policy.Trigger = "source_changed"
	q.Sources = append([]catalog.SourceRef(nil), q.Sources...)
	q.ClaimIDs = append([]string(nil), q.ClaimIDs...)
	q.Policy.Contexts = append([]jobs.ModelContextRef(nil), q.Policy.Contexts...)
	sort.Slice(q.Sources, func(i, j int) bool { return q.Sources[i].ID < q.Sources[j].ID })
	sort.Strings(q.ClaimIDs)
	sort.Slice(q.Policy.Contexts, func(i, j int) bool { return q.Policy.Contexts[i].SessionKey < q.Policy.Contexts[j].SessionKey })
	for i, r := range q.Sources {
		if r.ID == "" || r.Revision < 1 || r.AccessRevision < 1 || len(r.Hash) != 64 || i > 0 && q.Sources[i-1].ID == r.ID {
			return q, catalog.ErrInvalid
		}
	}
	for i, id := range q.ClaimIDs {
		if id == "" || len(id) > 1024 || i > 0 && q.ClaimIDs[i-1] == id {
			return q, catalog.ErrInvalid
		}
	}
	return q, nil
}

func completeRefs(sources []catalog.Source) []catalog.SourceRef {
	refs := make([]catalog.SourceRef, len(sources))
	for i, s := range sources {
		refs[i] = catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}

func buildEnvelope(sources []catalog.Source, selected []catalog.SourceRef, facts []catalog.AcceptedFact) (Envelope, error) {
	byID := map[string]catalog.Source{}
	for _, src := range sources {
		byID[src.ID] = src
	}
	out := Envelope{Version: Version, Units: []Unit{}}
	for _, ref := range selected {
		s, ok := byID[ref.ID]
		if !ok || s.Revision != ref.Revision || s.AccessRevision != ref.AccessRevision || s.Hash != ref.Hash {
			return out, catalog.ErrStale
		}
		u := Unit{Source: ref, ScopeType: s.ScopeType, ScopeID: s.ScopeID, EventTime: s.EventTime, Text: s.Body}
		u.ID = "unit_" + digest(u)
		out.Units = append(out.Units, u)
	}
	for _, f := range facts {
		s, ok := byID[f.Pin.Source.ID]
		if !ok {
			return out, catalog.ErrDenied
		}
		u := Unit{Source: f.Pin.Source, ScopeType: s.ScopeType, ScopeID: s.ScopeID, EventTime: s.EventTime, Text: string(f.Claim.Value), Accepted: &FactEvidence{Key: f.Claim.Key, MemoryType: f.Claim.MemoryType, Pin: f.Pin, Temporal: f.Temporal, Freshness: f.Freshness, Historical: f.Historical}}
		u.ID = "unit_" + digest(u)
		out.Units = append(out.Units, u)
	}
	sort.Slice(out.Units, func(i, j int) bool { return out.Units[i].ID < out.Units[j].ID })
	raw, _ := json.Marshal(out)
	if _, err := readEnvelope([]catalog.Source{{Body: string(raw)}}); err != nil {
		return out, err
	}
	// Reject a preparation that cannot retain every complete value even in the
	// largest (all-single) representation before admitting any paid inference.
	groups := Selection{Groups: []SelectedGroup{}}
	for _, u := range out.Units {
		groups.Groups = append(groups.Groups, SelectedGroup{Members: []string{u.ID}, Relation: "single"})
	}
	selectedJSON, _ := json.Marshal(groups)
	if _, err := (Handler{}).Validate([]catalog.Source{{Body: string(raw)}}, string(selectedJSON)); err != nil {
		return out, err
	}
	return out, nil
}

func pinsOf(in Envelope) []catalog.AcceptedFactPin {
	pins := []catalog.AcceptedFactPin{}
	for _, u := range in.Units {
		if u.Accepted != nil {
			pins = append(pins, u.Accepted.Pin)
		}
	}
	return pins
}

// Prepare creates one immutable evidence snapshot and one explicit finite owner
// policy in the same transaction. Retrying the identical policy returns its
// original receipt; changed evidence/decisions need a new policy identity.
func (s *Store) Prepare(ctx context.Context, request PrepareRequest) (jobs.ModelPolicy, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil || s.Jobs == nil {
		return jobs.ModelPolicy{}, catalog.ErrDenied
	}
	q, err := normalizePrepare(request)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	requestHash := digest(q)
	tx, err := s.Catalog.Pool().Begin(ctx)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return jobs.ModelPolicy{}, err
	}
	prior, err := readInput(ctx, tx, q.Policy.ID)
	if err == nil {
		if prior.OwnerID != actor.OwnerID || prior.RequestHash != requestHash {
			return jobs.ModelPolicy{}, catalog.ErrStale
		}
		var raw []byte
		var policy jobs.ModelPolicy
		if err = tx.QueryRow(ctx, `SELECT policy FROM memory_model_policies WHERE policy_id=$1 AND owner_id=$2`, q.Policy.ID, actor.OwnerID).Scan(&raw); err != nil || json.Unmarshal(raw, &policy) != nil {
			return jobs.ModelPolicy{}, catalog.ErrDenied
		}
		return policy, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return jobs.ModelPolicy{}, err
	}
	parents := append([]catalog.SourceRef(nil), q.Sources...)
	for _, id := range q.ClaimIDs {
		var ref catalog.SourceRef
		if err = tx.QueryRow(ctx, `SELECT source_id,source_revision FROM memory_claims WHERE claim_id=$1 AND owner_id=$2`, id, actor.OwnerID).Scan(&ref.ID, &ref.Revision); err != nil {
			return jobs.ModelPolicy{}, catalog.ErrDenied
		}
		parents = append(parents, ref)
	}
	q.Policy.Job.Sources = parents
	if _, err = s.Jobs.PrepareConsolidationContextsTx(ctx, tx, q.Policy); err != nil {
		return jobs.ModelPolicy{}, err
	}
	graph, err := s.Catalog.CanonicalSourcesTx(ctx, tx, parents)
	if err != nil || len(graph) > 63 {
		return jobs.ModelPolicy{}, catalog.ErrInvalid
	}
	current, err := s.Catalog.AccessTx(ctx, tx, graph, "memory_job_create")
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	parents = completeRefs(current)
	for _, src := range current {
		var derivedView bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_consolidation_inputs WHERE source_id=$1) OR EXISTS(SELECT 1 FROM memory_consolidation_generations WHERE source_id=$1)`, src.ID).Scan(&derivedView); err != nil {
			return jobs.ModelPolicy{}, err
		}
		if derivedView {
			return jobs.ModelPolicy{}, catalog.ErrInvalid
		}
	}
	page, err := s.Catalog.ReadAcceptedFactsTx(ctx, tx, q.ClaimIDs, q.Query)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if len(page.Items) != len(q.ClaimIDs) || page.Truncated || page.UnknownValidity > 0 || page.NotKnownAtObservation > 0 {
		return jobs.ModelPolicy{}, catalog.ErrStale
	}
	envelope, err := buildEnvelope(current, q.Sources, page.Items)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,89))`, actor.OwnerID); err != nil {
		return jobs.ModelPolicy{}, err
	}
	var retained, ownerRetained int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE view_key=$2),count(*) FROM memory_consolidation_inputs WHERE owner_id=$1 AND NOT cleanup_complete`, actor.OwnerID, q.ViewKey).Scan(&retained, &ownerRetained); err != nil {
		return jobs.ModelPolicy{}, err
	}
	if retained >= MaxRetainedVersions || ownerRetained >= MaxRetainedOwner {
		return jobs.ModelPolicy{}, jobs.ErrWaiting
	}
	body, _ := json.Marshal(envelope)
	locator, _ := json.Marshal(map[string]string{"consumer": Version, "view_key": q.ViewKey, "policy_id": q.Policy.ID})
	snapshot, err := s.Catalog.CreateDerivedTx(ctx, tx, parents, "memory_job_create", "Prepared consolidation evidence", string(body), locator)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if pins := pinsOf(envelope); len(pins) > 0 {
		if err = s.Catalog.BindNewAcceptedFactPinsTx(ctx, tx, snapshot, parents, pins, q.Query); err != nil {
			return jobs.ModelPolicy{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_consolidation_inputs(source_id,owner_id,view_key,policy_id,request_hash,source_units,claim_ids,temporal_query) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, snapshot.ID, actor.OwnerID, q.ViewKey, q.Policy.ID, requestHash, mustJSON(q.Sources), mustJSON(q.ClaimIDs), mustJSON(q.Query)); err != nil {
		return jobs.ModelPolicy{}, err
	}
	q.Policy.Job.Sources = []catalog.SourceRef{{ID: snapshot.ID, Revision: snapshot.Revision, AccessRevision: snapshot.AccessRevision, Hash: snapshot.Hash}}
	policy, err := s.Jobs.CreateModelPolicyTx(ctx, tx, q.Policy)
	if err != nil {
		return policy, err
	}
	return policy, tx.Commit(ctx)
}

func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
