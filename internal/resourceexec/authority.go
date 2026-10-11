package resourceexec

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *Store) load(ctx context.Context, tx pgx.Tx, ticket string) (dispatch, actionauth.Grant, error) {
	var d dispatch
	var g actionauth.Grant
	var raw []byte
	if !validTicket(ticket) {
		return d, g, ErrDenied
	}
	e := tx.QueryRow(ctx, `SELECT ticket_id,authorization_id,action_id,effect_id,owner_id,run_id,session_key,task_id,task_revision,constraints_revision,owner_lease_id,owner_epoch,resource_id,resource_lease_id,resource_version,resource_fence,profile_id,tool_name,contract_hash,args_hash,deadline,phase FROM execution_resource_dispatches WHERE ticket_id=$1`, ticket).Scan(&d.ticket, &d.authorization, &d.action, &d.effect, &d.binding.OwnerID, &d.binding.RunID, &d.binding.SessionKey, &d.binding.TaskID, &d.binding.TaskRevision, &d.binding.ConstraintsRevision, &d.ownerLease, &d.binding.LeaseEpoch, &d.proof.ResourceID, &d.proof.LeaseID, &d.proof.ExpectedResourceVersion, &d.proof.FenceEpoch, &d.profile, &d.tool, &d.contract, &d.args, &d.binding.Deadline, &d.phase)
	if e != nil {
		return d, g, ErrDenied
	}
	d.proof.RunID, d.proof.SessionKey, d.proof.TaskID = d.binding.RunID, d.binding.SessionKey, d.binding.TaskID
	e = tx.QueryRow(ctx, `SELECT grant_data FROM action_authorizations WHERE authorization_id=$1`, d.authorization).Scan(&raw)
	if e != nil || json.Unmarshal(raw, &g) != nil || g.ID != d.authorization || g.Binding.ActionID != d.action || g.Binding.RunID != d.binding.RunID || g.Binding.ToolName != d.tool || g.Binding.ContractHash != d.contract || g.Binding.ArgsHash != d.args || g.LeaseID != d.ownerLease || g.LeaseEpoch != d.binding.LeaseEpoch || g.Binding.OwnerID != d.binding.OwnerID || g.Binding.TaskID != d.binding.TaskID || g.Binding.TaskRevision != d.binding.TaskRevision || g.Binding.ConstraintsRevision != d.binding.ConstraintsRevision || s.profile.ID != d.profile || s.profile.ContractHashes[d.tool] != d.contract {
		return d, g, ErrDenied
	}
	return d, g, nil
}

// TicketForAction never accepts a model ticket and requires the already claimed
// permanent action/actual call. Missing hook composition is fail closed.
func (s *Store) TicketForAction(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, effect string) (string, error) {
	if call == nil || c == nil {
		return "", ErrDenied
	}
	ch, e := actionauth.ContractHash(c)
	if e != nil {
		return "", ErrDenied
	}
	ah, e := actionauth.ArgumentsHash(call.GetArgsJson())
	if e != nil {
		return "", ErrDenied
	}
	var ticket string
	e = s.pool.QueryRow(ctx, `SELECT ticket_id FROM execution_resource_dispatches WHERE effect_id=$1 AND run_id=$2 AND tool_name=$3 AND contract_hash=$4 AND args_hash=$5 AND phase='prepared' AND deadline>clock_timestamp()`, effect, call.GetRunId(), call.GetToolName(), ch, ah).Scan(&ticket)
	if e != nil {
		return "", ErrDenied
	}
	return ticket, nil
}

func (s *Store) Inspect(ctx context.Context, r RuntimeRequest) (RuntimeSpec, error) {
	return s.admit(ctx, r, false)
}
func (s *Store) Begin(ctx context.Context, r RuntimeRequest) (RuntimeSpec, error) {
	return s.admit(ctx, r, true)
}
func (s *Store) admit(ctx context.Context, r RuntimeRequest, begin bool) (RuntimeSpec, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return RuntimeSpec{}, e
	}
	defer tx.Rollback(context.Background())
	d, g, e := s.load(ctx, tx, r.Ticket)
	if e != nil {
		return RuntimeSpec{}, e
	}
	if r.RunID != d.binding.RunID || r.ToolCallID != g.Binding.ToolCallID || r.ToolName != d.tool || r.ActionID != d.effect || r.ContractHash != d.contract || r.ArgsHash != d.args || d.phase != "prepared" {
		return RuntimeSpec{}, ErrDenied
	}
	ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: d.binding.SessionKey, RunID: d.binding.RunID, LeaseID: d.ownerLease, FenceEpoch: d.binding.LeaseEpoch})
	if e = s.checkAuthority(ctx, tx, g, d.binding); e != nil {
		return RuntimeSpec{}, e
	}
	if _, e = s.sessionTx(ctx, tx, d.binding, d.proof.ResourceID, d.tool, false); e != nil {
		return RuntimeSpec{}, e
	}
	if e = s.lockJournal(ctx, tx, d, r.Fingerprint); e != nil {
		return RuntimeSpec{}, e
	}
	if e = s.checkCurrent(ctx, tx, g, d.binding, d.proof, d.effect); e != nil {
		return RuntimeSpec{}, e
	}
	if begin {
		tag, err := tx.Exec(ctx, `UPDATE execution_resource_dispatches SET phase='running' WHERE ticket_id=$1 AND phase='prepared' AND deadline>clock_timestamp()`, d.ticket)
		if err != nil || tag.RowsAffected() != 1 {
			return RuntimeSpec{}, ErrDenied
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return RuntimeSpec{}, e
	}
	return RuntimeSpec{Lane: d.proof.ResourceID, Fence: d.proof.FenceEpoch, Deadline: d.binding.Deadline}, nil
}

// Match outcome recording's effect-before-grant/dispatch order. Stabilize the
// actual inflight journal before resource rows; a delayed ticket cannot start
// after the original Broker has already recorded its uncertain outcome.
func (s *Store) lockJournal(ctx context.Context, tx pgx.Tx, d dispatch, fingerprint string) error {
	var live bool
	e := tx.QueryRow(ctx, `SELECT state='executing' AND authorization_id=$2 AND fingerprint=$3 FROM action_effects WHERE action_id=$1 FOR SHARE`, d.effect, d.authorization, fingerprint).Scan(&live)
	if e != nil || !live {
		return ErrDenied
	}
	e = tx.QueryRow(ctx, `SELECT a.state='inflight' AND a.expires_at>clock_timestamp() AND i.expires_at>clock_timestamp() AND x.outcome='executing' AND x.action_id=$2 AND x.execution_action_id=$3
 FROM action_authorizations a JOIN action_intents i ON i.action_id=a.action_id JOIN action_effect_dispatches x ON x.authorization_id=a.authorization_id WHERE a.authorization_id=$1 AND i.effect_id=$2 FOR SHARE OF a,i,x`, d.authorization, d.effect, d.action).Scan(&live)
	if e != nil || !live {
		return ErrDenied
	}
	return nil
}

func (s *Store) Finish(ctx context.Context, r Receipt) error {
	if !validTicket(r.Ticket) || len(r.Digest) != 64 || !isHex(r.Digest) {
		return ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	d, g, e := s.load(ctx, tx, r.Ticket)
	if e != nil {
		return e
	}
	if d.phase != "running" || r.RunID != d.binding.RunID || r.ToolCallID != g.Binding.ToolCallID || r.ToolName != d.tool || r.ActionID != d.effect || r.ContractHash != d.contract || r.ArgsHash != d.args {
		return ErrDenied
	}
	var fingerprint string
	if e = tx.QueryRow(ctx, `SELECT fingerprint FROM action_effects WHERE action_id=$1`, d.effect).Scan(&fingerprint); e != nil || fingerprint != r.Fingerprint {
		return ErrDenied
	}
	phase := "unknown"
	if r.Completed {
		ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: d.binding.SessionKey, RunID: d.binding.RunID, LeaseID: d.ownerLease, FenceEpoch: d.binding.LeaseEpoch})
		if e = s.checkAuthority(ctx, tx, g, d.binding); e != nil {
			return e
		}
		if _, e = s.sessionTx(ctx, tx, d.binding, d.proof.ResourceID, d.tool, d.tool == "browser.close"); e != nil {
			return e
		}
		if e = s.lockJournal(ctx, tx, d, r.Fingerprint); e != nil {
			return e
		}
		if e = s.checkCurrent(ctx, tx, g, d.binding, d.proof, d.effect); e != nil {
			return e
		}
		if _, e = s.resources.CompleteTx(ctx, tx, d.binding, d.proof, "lane_"+r.Digest); e != nil {
			return e
		}
		if d.tool == "browser.open" {
			var at time.Time
			if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
				return e
			}
			if len(r.SessionID) != 32 || !isHex(r.SessionID) || !r.SessionExpiresAt.After(at) || r.SessionExpiresAt.After(at.Add(15*time.Minute)) {
				return ErrDenied
			}
			h := hash(s.profile.ID + "\x00" + r.SessionID)
			_, e = tx.Exec(ctx, `INSERT INTO execution_browser_resources(profile_id,session_hash,resource_id,owner_id,run_id,task_id,task_revision,created_by_effect,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s.profile.ID, h, "browser-tab:"+h, d.binding.OwnerID, d.binding.RunID, d.binding.TaskID, d.binding.TaskRevision, d.effect, r.SessionExpiresAt)
			if e != nil {
				return e
			}
		} else if d.tool == "browser.close" {
			if len(r.SessionID) != 32 || !isHex(r.SessionID) || "browser-tab:"+hash(s.profile.ID+"\x00"+r.SessionID) != d.proof.ResourceID {
				return ErrDenied
			}
			tag, err := tx.Exec(ctx, `UPDATE execution_browser_resources SET closed_at=clock_timestamp() WHERE profile_id=$1 AND session_hash=$2 AND closed_at IS NULL`, s.profile.ID, strings.TrimPrefix(d.proof.ResourceID, "browser-tab:"))
			if err != nil || tag.RowsAffected() != 1 {
				return ErrDenied
			}
		}
		phase = "completed"
	}
	tag, e := tx.Exec(ctx, `UPDATE execution_resource_dispatches SET phase=$2,receipt_hash=$3,finished_at=clock_timestamp() WHERE ticket_id=$1 AND phase='running'`, d.ticket, phase, r.Digest)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return tx.Commit(ctx)
}
