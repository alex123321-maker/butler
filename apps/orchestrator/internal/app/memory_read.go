package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemoryReads is the owning service boundary. The permanent Broker dispatch
// supplies identity; model arguments can select only bounded exact DATA reads.
type MemoryReads struct {
	pool         *pgxpool.Pool
	sources      *catalog.Store
	leases       *session.PostgresLeaseManager
	inputs       *durableinput.Store
	hashes       map[string]string
	adaptive     *adaptive.Store
	semantic     memoryservice.PolicySemanticReader
	temporal     bool
	sourceSubset []catalog.SourceRef
	experience   *experience.Store
}

// AdaptiveReadOptions is trusted composition, never a request/model flag.
// Tests may explicitly compose an unqualified policy without enabling it in a
// running deployment. Semantic uses only the existing guarded local reader.
type AdaptiveReadOptions struct {
	Store      *adaptive.Store
	Semantic   memoryservice.PolicySemanticReader
	Experience *experience.Store
	// Unexported trusted evaluation cohort. It adds denial only and cannot be
	// supplied through a Broker envelope, stored flag or model request.
	sourceSubset []catalog.SourceRef
}

func NewMemoryReads(pool *pgxpool.Pool, sources *catalog.Store, options ...AdaptiveReadOptions) (*MemoryReads, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, readcontrol.ErrDenied
	}
	s := &MemoryReads{pool: pool, sources: sources, leases: session.NewPostgresLeaseManager(pool), inputs: durableinput.NewStore(pool), hashes: map[string]string{}}
	if len(options) > 1 {
		return nil, readcontrol.ErrDenied
	}
	if len(options) == 1 {
		s.adaptive = options[0].Store
		s.semantic = options[0].Semantic
		s.experience = options[0].Experience
		if options[0].sourceSubset != nil {
			if len(options[0].sourceSubset) > 8 {
				return nil, readcontrol.ErrDenied
			}
			s.sourceSubset = append([]catalog.SourceRef{}, options[0].sourceSubset...)
			for _, ref := range s.sourceSubset {
				if ref.ID == "" || ref.Revision < 1 || ref.AccessRevision < 1 || len(ref.Hash) != 64 {
					return nil, readcontrol.ErrDenied
				}
			}
		}
	}
	for _, c := range append(readcontrol.Contracts(), factcontrol.Contract()) {
		h, e := actionauth.ContractHash(c)
		if e != nil {
			return nil, e
		}
		s.hashes[c.ToolName] = h
	}
	return s, nil
}
func (s *MemoryReads) CheckReady(ctx context.Context) error {
	if s == nil {
		return readcontrol.ErrDenied
	}
	return s.sources.CheckRestore(ctx)
}
func (s *MemoryReads) ReadDispatched(ctx context.Context, e readcontrol.Envelope) (readcontrol.Result, error) {
	a, err := readcontrol.DecodeArgs(e.Tool, e.ArgsJSON)
	var facts *factcontrol.Args
	if e.Tool == factcontrol.Tool && s != nil && s.temporal {
		value, decodeErr := factcontrol.DecodeArgs(e.ArgsJSON)
		facts = &value
		err = decodeErr
	}
	if s == nil || err != nil || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || e.ContractHash != s.hashes[e.Tool] {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if s.adaptive == nil && (a.Strategy == "semantic_hybrid" || a.EventFrom != "" || a.EventUntil != "") {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	envelope := delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}
	contract := readcontrol.Contract(e.Tool)
	if facts != nil {
		contract = factcontrol.Contract()
	}
	read := func(ctx context.Context, q admissionQuery, lock bool) (actionauth.Grant, error) {
		return readAdmissionDispatchRevision(ctx, q, envelope, e.Tool, s.hashes[e.Tool], contract.GetContractRevision(), actionauth.EffectRead, lock)
	}
	grant, err := read(ctx, s.pool, false)
	if err != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	var key string
	var expiry time.Time
	if s.pool.QueryRow(ctx, `SELECT r.session_key,o.expires_at FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=r.run_id AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()`, e.RunID, grant.LeaseID, grant.LeaseEpoch).Scan(&key, &expiry) != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	var claim durableinput.Record
	if s.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, e.RunID, key, grant.Binding.OwnerID, grant.Binding.TaskID).Scan(&claim.InputID, &claim.ClaimID) != nil || grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if grant.ExpiresAt.Before(expiry) {
		expiry = grant.ExpiresAt
	}
	deadline, _ := ctx.Deadline()
	base, stop := context.WithDeadline(context.Background(), deadline)
	forward := context.AfterFunc(ctx, stop)
	defer stop()
	defer forward()
	token := ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch}
	owned := ownership.WithToken(durableinput.WithClaim(base, claim), s.leases.Authority(), token)
	binding := resources.Binding{OwnerID: grant.Binding.OwnerID, SessionKey: key, RunID: e.RunID, TaskID: grant.Binding.TaskID, TaskRevision: grant.Binding.TaskRevision, ConstraintsRevision: grant.Binding.ConstraintsRevision, LeaseEpoch: grant.LeaseEpoch, Deadline: expiry}
	check := func(tx pgx.Tx) error {
		if (resources.WorkerGuard{}).CheckTx(owned, tx, binding) != nil || s.inputs.CheckDispatchClaimTx(owned, tx, e.RunID, e.ToolCallID, e.EffectID, grant.ID) != nil {
			return readcontrol.ErrDenied
		}
		current, err := read(owned, tx, true)
		if err != nil || current.ID != grant.ID || current.InputClaimHash != grant.InputClaimHash || current.Binding.ActionHash != grant.Binding.ActionHash {
			return readcontrol.ErrDenied
		}
		return s.sources.CheckRunSourcesTx(owned, tx, e.RunID)
	}
	var candidates []reader.SemanticCandidate
	var diagnostics []string
	var lessonEnvironment experience.Prepared
	if s.experience != nil {
		lessonEnvironment, err = s.experience.Prepare(owned, e.RunID)
		if err != nil {
			return readcontrol.Empty("denied"), err
		}
	}
	if s.adaptive != nil {
		// Check exact permanent dispatch before spending the conservative read
		// reservation. No owning read transaction spans semantic IPC.
		proof, err := s.pool.Begin(owned)
		if err != nil {
			return readcontrol.Empty("backend_error"), err
		}
		err = check(proof)
		rollbackErr := proof.Rollback(owned)
		if err != nil || rollbackErr != nil {
			return readcontrol.Empty("denied"), readcontrol.ErrDenied
		}
		if _, err = s.adaptive.Reserve(owned, e, a); err != nil {
			return readcontrol.Empty("denied"), err
		}
		if e.Tool == readcontrol.Search && a.Strategy == "semantic_hybrid" {
			if s.semantic == nil {
				return readcontrol.Empty("backend_error"), readcontrol.ErrDenied
			}
			scopes := []memoryservice.Scope{{Type: "session", ID: key}, {Type: "user", ID: grant.Binding.OwnerID}, {Type: "global", ID: "global"}}
			extra, err := s.sources.ExtraScopes(owned, "memory_read")
			if err != nil {
				return readcontrol.Empty("denied"), err
			}
			seen := map[memoryservice.Scope]bool{}
			for _, scope := range scopes {
				seen[scope] = true
			}
			for _, scope := range extra {
				next := memoryservice.Scope{Type: scope.Type, ID: scope.ID}
				if !seen[next] {
					scopes = append(scopes, next)
					seen[next] = true
				}
			}
			if len(scopes) > 12 {
				return readcontrol.Empty("denied"), readcontrol.ErrDenied
			}
			found, err := s.semantic.SearchWithPolicy(owned, scopes, a.Query, a.Limit, memoryservice.QueryPolicy{Local: true, External: false})
			if err != nil {
				return readcontrol.Empty("backend_error"), err
			}
			if len(found.Hits) > 4 || len(found.Diagnostics) > 8 {
				return readcontrol.Empty("backend_error"), readcontrol.ErrDenied
			}
			for _, hit := range found.Hits {
				id, err := strconv.ParseInt(strings.TrimPrefix(hit.SourceRef, "memory_chunk:"), 10, 64)
				revision, timeErr := time.Parse(time.RFC3339Nano, hit.Revision)
				if !strings.HasPrefix(hit.SourceRef, "memory_chunk:") || err != nil || id <= 0 || timeErr != nil {
					return readcontrol.Empty("backend_error"), readcontrol.ErrDenied
				}
				candidates = append(candidates, reader.SemanticCandidate{ID: id, Revision: revision})
			}
			diagnostics = found.Diagnostics
		}
	}
	tx, err := s.pool.Begin(owned)
	if err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	defer tx.Rollback(context.Background())
	if check(tx) != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	var result readcontrol.Result
	if facts != nil {
		result, err = s.readTemporalFactsTx(owned, tx, *facts, lessonEnvironment)
	} else {
		result, err = (reader.Reader{Sources: s.sources, SemanticCandidates: candidates, SourceSubset: s.sourceSubset, SourceEligibility: func(ctx context.Context, tx pgx.Tx, src catalog.Source) (bool, error) {
			return s.experience.EligibleTx(ctx, tx, src, lessonEnvironment, true)
		}}).ReadTx(owned, tx, e.Tool, a)
	}
	if err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	// Repeat after all source/fragment lock waits; a result cannot outlive its
	// exact task, input, lease, source ancestor or deletion watermark.
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if s.temporal && facts == nil {
		if err = s.enrichTemporalReadTx(owned, tx, &result); err != nil {
			return readcontrol.Empty("denied"), err
		}
	}
	if s.adaptive != nil {
		if err = s.adaptive.ObserveTx(owned, tx, e, a, &result); err != nil {
			return readcontrol.Empty("denied"), err
		}
	}
	if check(tx) != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if err = s.experience.CheckPinnedTx(owned, tx, e.RunID); err != nil {
		return readcontrol.Empty("denied"), err
	}
	if err = tx.Commit(owned); err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	return result, nil
}
