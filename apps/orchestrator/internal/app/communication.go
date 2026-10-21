package app

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"strings"
	"time"
)

// CommunicationControl is deliberately not installed by Bootstrap. A future
// channel rollout must explicitly compose it and pin kernel admissions first.
type CommunicationControl struct{ Store *communication.Store }

func (s *CommunicationControl) ExecuteDispatched(ctx context.Context, e communicationcontrol.Envelope) (communicationcontrol.Receipt, error) {
	var out communicationcontrol.Receipt
	if s == nil || s.Store == nil || s.Store.SourceGuard == nil {
		return out, communication.ErrDenied
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return out, communication.ErrDenied
	}
	if original, err := actionauth.ArgumentsHash(e.ArgsJSON); err != nil {
		return out, communication.ErrDenied
	} else if safe, err := actionauth.ArgumentsHash(sanitize.JSON(e.ArgsJSON)); err != nil || safe != original {
		return out, communication.ErrDenied
	}
	if _, err := communicationcontrol.DecodeArgs(e.Tool, e.ArgsJSON); err != nil {
		return out, err
	}
	contract := communicationcontrol.Contract(e.Tool)
	hash, err := actionauth.ContractHash(contract)
	if err != nil || hash != e.ContractHash {
		return out, communication.ErrDenied
	}
	envelope := delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grant, err := readOwnedDispatch(ctx, s.Store.Pool, envelope, e.Tool, hash, "1", actionauth.EffectPrepare, false, true)
	if err != nil {
		return out, communication.ErrDenied
	}
	var key string
	var claim durableinput.Record
	if s.Store.Pool.QueryRow(ctx, `SELECT session_key,input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND owner_id=$2 AND status='running' AND claim_expires_at>clock_timestamp()`, e.RunID, grant.Binding.OwnerID).Scan(&key, &claim.InputID, &claim.ClaimID) != nil || grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return out, communication.ErrDenied
	}
	leases := session.NewPostgresLeaseManager(s.Store.Pool)
	// Do not inherit arbitrary caller identity/authority values from a private RPC.
	deadline, _ := ctx.Deadline()
	base, stop := context.WithDeadline(context.Background(), deadline)
	defer stop()
	forward := context.AfterFunc(ctx, stop)
	defer forward()
	owned := ownership.WithToken(durableinput.WithClaim(base, claim), leases.Authority(), ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch})
	owned, release, err := ownership.Begin(owned)
	if err != nil {
		return out, communication.ErrDenied
	}
	defer release()
	tx, err := s.Store.Pool.Begin(owned)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	var owner string
	if tx.QueryRow(owned, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, key).Scan(&owner) != nil || owner != grant.Binding.OwnerID {
		return out, communication.ErrDenied
	}
	if !e.Lookup {
		current, err := readOwnedDispatch(owned, tx, envelope, e.Tool, hash, "1", actionauth.EffectPrepare, true, true)
		if err != nil || current.ID != grant.ID {
			return out, communication.ErrDenied
		}
		if s.Store.Inputs.CheckDispatchClaimTx(owned, tx, e.RunID, e.ToolCallID, e.EffectID, grant.ID) != nil {
			return out, communication.ErrDenied
		}
	}
	p := communication.Principal{DestinationsHash: grant.Binding.CommunicationDestinationsHash, OwnerID: owner, SessionKey: key, RunID: e.RunID, InputID: claim.InputID, TaskID: grant.Binding.TaskID, TaskRevision: grant.Binding.TaskRevision, ConstraintsRevision: grant.Binding.ConstraintsRevision}
	if !e.Lookup {
		a, _ := communicationcontrol.DecodeArgs(e.Tool, e.ArgsJSON)
		for _, ref := range a.SourceRefs {
			var selected bool
			if tx.QueryRow(owned, `SELECT EXISTS(SELECT 1 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2)`, e.RunID, strings.TrimPrefix(ref, "source:")).Scan(&selected) != nil || !selected {
				return out, communication.ErrDenied
			}
		}
	}
	if e.Lookup {
		out, err = s.Store.LookupTx(owned, tx, p, e)
	} else {
		out, err = s.Store.ApplyTx(owned, tx, p, e)
	}
	if err != nil {
		return out, err
	}
	return out, tx.Commit(owned)
}
