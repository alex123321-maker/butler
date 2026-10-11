package workspaceexec

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/metadata"
)

const TicketHeader = "butler-workspace-ticket"

type ticketKey struct{}

// Only the private Broker router carries this value. It is never serialized in
// a ToolCall, model-visible context or result.
func WithTicket(ctx context.Context, ticket string) context.Context {
	return context.WithValue(ctx, ticketKey{}, ticket)
}
func OutgoingContext(ctx context.Context) context.Context {
	if ticket, ok := ctx.Value(ticketKey{}).(string); ok && len(ticket) == 48 {
		return metadata.AppendToOutgoingContext(ctx, TicketHeader, ticket)
	}
	return ctx
}

type dispatch struct {
	ticket, effect, phase string
	prepared              prepared
	grant                 actionauth.Grant
	proof                 resources.Proof
	deadline              time.Time
	ownCompletion         bool
}

func (s *DispatchStore) load(ctx context.Context, tx pgx.Tx, ticket string) (dispatch, error) {
	var d dispatch
	if len(ticket) != 48 {
		return d, ErrDenied
	}
	var authorization string
	err := tx.QueryRow(ctx, `SELECT ticket_id,authorization_id,effect_id,resource_id,resource_lease_id,resource_version,resource_fence,deadline,phase,xmin=pg_current_xact_id()::xid FROM code_workspace_dispatches WHERE ticket_id=$1`, ticket).Scan(&d.ticket, &authorization, &d.effect, &d.proof.ResourceID, &d.proof.LeaseID, &d.proof.ExpectedResourceVersion, &d.proof.FenceEpoch, &d.deadline, &d.phase, &d.ownCompletion)
	if err != nil {
		return d, ErrDenied
	}
	d.prepared, err = s.loadPrepared(ctx, tx, authorization)
	if err != nil {
		return d, err
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT grant_data FROM action_authorizations WHERE authorization_id=$1`, authorization).Scan(&raw) != nil || json.Unmarshal(raw, &d.grant) != nil {
		return d, ErrDenied
	}
	p, g := d.prepared, d.grant
	b := p.source.ref.Binding
	if g.ID != authorization || g.Binding.ToolName != p.tool || g.Binding.ContractHash != p.contract || g.Binding.ArgsHash != p.args || g.InputClaimHash != p.input || g.Binding.RunID != p.source.run || g.Binding.OwnerID != b.OwnerID || g.Binding.TaskID != b.TaskID || g.Binding.TaskRevision != b.TaskRevision || g.Binding.ConstraintsRevision != b.ConstraintsRevision || d.proof.ResourceID != workspace.ResourceID(p.source.ref.ID) {
		return d, ErrDenied
	}
	d.proof.RunID, d.proof.SessionKey, d.proof.TaskID = p.source.run, b.SessionKey, b.TaskID
	return d, nil
}
func (s *DispatchStore) TicketForAction(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, effect string) (string, error) {
	if s == nil || call == nil || !ExactContract(c) || c.GetRuntimeTarget() != s.profile.Target {
		return "", ErrDenied
	}
	ch, err := actionauth.ContractHash(c)
	if err != nil {
		return "", err
	}
	ah, err := actionauth.ArgumentsHash(call.GetArgsJson())
	if err != nil {
		return "", err
	}
	var ticket string
	err = s.pool.QueryRow(ctx, `SELECT d.ticket_id FROM code_workspace_dispatches d JOIN code_workspace_authorizations a USING(authorization_id) JOIN action_authorizations g USING(authorization_id) JOIN action_intents i ON i.action_id=g.action_id WHERE d.effect_id=$1 AND i.run_id=$2 AND i.tool_call_id=$3 AND a.tool_name=$4 AND a.contract_hash=$5 AND a.args_hash=$6 AND a.profile_id=$7 AND d.phase='prepared' AND d.deadline>clock_timestamp()`, effect, call.GetRunId(), call.GetToolCallId(), call.GetToolName(), ch, ah, s.profile.ID).Scan(&ticket)
	if err != nil {
		return "", ErrDenied
	}
	return ticket, nil
}

func (s *DispatchStore) lockJournal(ctx context.Context, tx pgx.Tx, d dispatch, fingerprint string) error {
	var live bool
	if tx.QueryRow(ctx, `SELECT state='executing' AND authorization_id=$2 AND ($3='' OR fingerprint=$3) FROM action_effects WHERE action_id=$1 FOR SHARE`, d.effect, d.grant.ID, fingerprint).Scan(&live) != nil || !live {
		return ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT a.state='inflight' AND a.expires_at>clock_timestamp() AND i.expires_at>clock_timestamp() AND i.action_hash=$4 AND i.effect_id=$2 AND x.outcome='executing' AND x.action_id=$2 AND x.execution_action_id=$3 FROM action_authorizations a JOIN action_intents i ON i.action_id=a.action_id JOIN action_effect_dispatches x ON x.authorization_id=a.authorization_id WHERE a.authorization_id=$1 FOR SHARE OF a,i,x`, d.grant.ID, d.effect, d.grant.Binding.ActionID, d.grant.Binding.ActionHash).Scan(&live) != nil || !live {
		return ErrDenied
	}
	return nil
}
func (s *DispatchStore) checkCurrent(ctx context.Context, tx pgx.Tx, d dispatch, b resources.Binding, fingerprint string) error {
	if _, err := checkInputTx(ctx, tx, d.grant, b, d.effect); err != nil {
		return err
	}
	if err := s.provenance.CheckTx(ctx, tx, d.prepared.source, d.proof.LeaseID); err != nil {
		return err
	}
	if err := s.lockJournal(ctx, tx, d, fingerprint); err != nil {
		return err
	}
	deadline, err := s.budgets.ToolDeadlineTx(ctx, tx, b.OwnerID, d.effect)
	if err != nil || deadline.Before(d.deadline) {
		return ErrDenied
	}
	mode := resources.Write
	if d.prepared.tool == ReadTool {
		mode = resources.Read
	}
	if err = s.resources.CheckTx(ctx, tx, b, d.proof, mode); err != nil {
		return err
	}
	// Neither a resource/lease wait nor an appended anti-rollback anchor may
	// extend the original input/source authority used before that wait.
	if _, err = checkInputTx(ctx, tx, d.grant, b, d.effect); err != nil {
		return err
	}
	return s.provenance.CheckTx(ctx, tx, d.prepared.source, d.proof.LeaseID)
}

func (s *DispatchStore) begin(ctx context.Context, req *runtimev1.ExecuteRequest, ticket string) (context.Context, dispatch, Args, func(), error) {
	var empty dispatch
	call, c, ec := req.GetToolCall(), req.GetContract(), req.GetContext()
	if s == nil || call == nil || !ExactContract(c) || c.GetRuntimeTarget() != s.profile.Target || ec == nil || call.GetRunId() != ec.GetRunId() || call.GetToolCallId() != ec.GetToolCallId() || call.GetToolName() != ec.GetToolName() || ec.GetRuntimeTarget() != s.profile.Target || ec.GetActionFingerprint() == "" || len(call.GetCredentialRefs()) != 0 || len(req.GetResolvedCredentials()) != 0 || call.GetRuntimeTarget() != "" && call.GetRuntimeTarget() != s.profile.Target {
		return nil, empty, Args{}, nil, ErrDenied
	}
	a, err := DecodeArgs(call.GetToolName(), call.GetArgsJson())
	if err != nil {
		return nil, empty, Args{}, nil, err
	}
	ch, err := actionauth.ContractHash(c)
	if err != nil {
		return nil, empty, Args{}, nil, err
	}
	ah, err := actionauth.ArgumentsHash(call.GetArgsJson())
	if err != nil {
		return nil, empty, Args{}, nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, empty, Args{}, nil, err
	}
	defer tx.Rollback(context.Background())
	d, err := s.load(ctx, tx, ticket)
	if err != nil || d.phase != "prepared" || d.effect != ec.GetActionId() || d.grant.Binding.RunID != call.GetRunId() || d.grant.Binding.ToolCallID != call.GetToolCallId() || d.prepared.tool != call.GetToolName() || d.prepared.contract != ch || d.prepared.args != ah || d.prepared.path != a.Path || d.prepared.expected != a.Expected || d.prepared.tool != CreateTool && d.prepared.source.ref.ID != a.WorkspaceID || d.prepared.tool == WriteTool && (d.prepared.bodyHash != bodyDigest(a.Content) || d.prepared.bytes != int64(len(a.Content))) {
		return nil, empty, Args{}, nil, ErrDenied
	}
	owned, b, deadline, stop, err := s.ownedTx(ctx, tx, d.grant)
	if err != nil {
		return nil, empty, Args{}, nil, err
	}
	if d.deadline.After(deadline) {
		stop()
		return nil, empty, Args{}, nil, ErrDenied
	}
	b.Deadline = d.deadline
	if err = s.checkCurrent(owned, tx, d, b, ec.GetActionFingerprint()); err != nil {
		stop()
		return nil, empty, Args{}, nil, err
	}
	tag, err := tx.Exec(owned, `UPDATE code_workspace_dispatches SET phase='running' WHERE ticket_id=$1 AND phase='prepared' AND deadline>clock_timestamp()`, ticket)
	if err != nil || tag.RowsAffected() != 1 {
		stop()
		return nil, empty, Args{}, nil, ErrDenied
	}
	if err = tx.Commit(owned); err != nil {
		stop()
		return nil, empty, Args{}, nil, err
	}
	d.phase = "running"
	return context.WithValue(owned, ticketKey{}, ticket), d, a, stop, nil
}

// The actual storage transaction calls this before touching files and again
// after waits. No RPC or second pool is borrowed while its locks are held.
func (s *DispatchStore) CheckWorkspaceTx(ctx context.Context, tx pgx.Tx, a workspace.Access) (resources.Binding, error) {
	ticket, _ := ctx.Value(ticketKey{}).(string)
	d, err := s.load(ctx, tx, ticket)
	if err != nil {
		return resources.Binding{}, err
	}
	p := d.prepared
	operation := "write"
	if p.tool == CreateTool {
		operation = "create"
	}
	if p.tool == ReadTool {
		operation = "read"
	}
	proof := d.proof
	if a.Ref != p.source.ref || a.Operation != operation || a.Path != p.path || a.Proof.LeaseID != proof.LeaseID || a.Proof.ResourceID != proof.ResourceID || a.Proof.FenceEpoch != proof.FenceEpoch || a.Proof.ExpectedResourceVersion != proof.ExpectedResourceVersion || a.Proof.RunID != proof.RunID || a.Proof.SessionKey != proof.SessionKey || a.Proof.TaskID != proof.TaskID || operation == "write" && (a.ContentSHA256 != p.bodyHash || a.Bytes != p.bytes) || d.phase != "running" && !(d.phase == "completed" && d.ownCompletion) {
		return resources.Binding{}, ErrDenied
	}
	owned, b, deadline, stop, err := s.ownedTx(ctx, tx, d.grant)
	if err != nil {
		return b, err
	}
	defer stop()
	if d.deadline.After(deadline) {
		return b, ErrDenied
	}
	b.Deadline = d.deadline
	if err = s.checkCurrent(owned, tx, d, b, ""); err != nil {
		return b, err
	}
	return b, nil
}
