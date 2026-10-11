package workspaceexec

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"time"
	"unicode/utf8"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/metadata"
)

// Runtime owns actual bounded files and privacy erasure. Broker admission and
// immutable dispatch remain mandatory; this service plans no run or tool.
type Runtime struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	dispatch *DispatchStore
	files    *workspace.Store
	observer *workspace.Observer
	purge    *workspace.DeletionRuntime
	token    string
}

func NewRuntime(ctx context.Context, d *DispatchStore, root, token string) (*Runtime, error) {
	if d == nil || len(token) < 32 || len(token) > 4096 || d.CheckReady(ctx) != nil {
		return nil, ErrDenied
	}
	o, err := workspace.NewObserver(root, MaxCodeBytes)
	if err != nil {
		return nil, err
	}
	r := &Runtime{dispatch: d, observer: o, token: token}
	g, err := workspace.NewResourceGuard(d.resources, d, r)
	if err != nil {
		o.Close()
		return nil, err
	}
	limits := workspace.DefaultLimits()
	limits.MaxFileBytes = MaxCodeBytes
	r.files, err = workspace.New(d.pool, root, g, limits)
	if err != nil {
		o.Close()
		return nil, err
	}
	r.purge, err = workspace.NewDeletionRuntime(root)
	if err != nil {
		r.files.Close()
		o.Close()
		return nil, err
	}
	return r, nil
}
func (r *Runtime) Close() error {
	first := r.files.Close()
	if err := r.observer.Close(); first == nil {
		first = err
	}
	if err := r.purge.Close(); first == nil {
		first = err
	}
	return first
}
func (r *Runtime) RecordNamespaceTx(ctx context.Context, tx pgx.Tx, a workspace.Access) (string, error) {
	o, err := r.observer.NamespaceTx(ctx, tx, a)
	if err != nil {
		return "", err
	}
	return r.record(ctx, tx, a, o)
}
func (r *Runtime) RecordWriteTx(ctx context.Context, tx pgx.Tx, a workspace.Access, s workspace.Snapshot) (string, error) {
	o, err := r.observer.FileTx(ctx, tx, a)
	if err != nil || o.Snapshot != s {
		return "", workspace.ErrDrift
	}
	return r.record(ctx, tx, a, o)
}
func (r *Runtime) record(ctx context.Context, tx pgx.Tx, a workspace.Access, o workspace.Observation) (string, error) {
	ticket, _ := ctx.Value(ticketKey{}).(string)
	d, err := r.dispatch.load(ctx, tx, ticket)
	if err != nil || d.phase != "running" {
		return "", ErrDenied
	}
	source, err := r.dispatch.provenance.ConfirmTx(ctx, tx, d.prepared.source, a, o)
	if err != nil {
		return "", err
	}
	if err = r.saveEvidence(ctx, tx, ticket, source, o); err != nil {
		return "", err
	}
	return "workspace:" + o.Digest, nil
}
func (r *Runtime) saveEvidence(ctx context.Context, tx pgx.Tx, ticket, source string, o workspace.Observation) error {
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO code_workspace_effect_evidence(ticket_id,source_id,observation,observation_hash) VALUES($1,$2,$3,$4)`, ticket, source, raw, o.Digest); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE code_workspace_dispatches SET phase='completed',finished_at=clock_timestamp() WHERE ticket_id=$1 AND phase='running' AND deadline>clock_timestamp()`, ticket)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return nil
}

func privateProof(p dispatch) workspace.LeaseProof {
	x := p.proof
	return workspace.LeaseProof{LeaseID: x.LeaseID, ResourceID: x.ResourceID, FenceEpoch: x.FenceEpoch, ExpectedResourceVersion: x.ExpectedResourceVersion, RunID: x.RunID, SessionKey: x.SessionKey, TaskID: x.TaskID}
}
func (r *Runtime) finishRead(ctx context.Context, d dispatch, f workspace.File) error {
	tx, err := r.dispatch.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	actual, err := r.dispatch.load(ctx, tx, d.ticket)
	if err != nil || actual.phase != "running" {
		return ErrDenied
	}
	owned, b, deadline, stop, err := r.dispatch.ownedTx(ctx, tx, actual.grant)
	if err != nil {
		return err
	}
	defer stop()
	if actual.deadline.After(deadline) {
		return ErrDenied
	}
	b.Deadline = actual.deadline
	if err = r.dispatch.checkCurrent(owned, tx, actual, b, ""); err != nil {
		return err
	}
	var version, bytes int64
	var digest string
	err = tx.QueryRow(owned, `SELECT version,content_sha256,byte_length FROM code_workspace_files WHERE workspace_id=$1 AND path=$2`, actual.prepared.source.ref.ID, f.Path).Scan(&version, &digest, &bytes)
	if err == pgx.ErrNoRows && f.Version == 0 {
		err = nil
		version = 0
		digest = ""
		bytes = 0
	}
	if err != nil || version != f.Version || digest != f.SHA256 || bytes != f.Bytes {
		return workspace.ErrConflict
	}
	// The actual kernel read already independently compared physical bytes and
	// metadata under its source/resource transaction. A held read lease prevents
	// another sanctioned writer until this current final check releases it.
	encoded, _ := json.Marshal([]any{actual.prepared.source.ref.ID, "file_read", f.Snapshot})
	o := workspace.Observation{WorkspaceID: actual.prepared.source.ref.ID, Kind: "file_read", Snapshot: f.Snapshot, Digest: bodyDigest(encoded)}
	if err = r.saveEvidence(owned, tx, actual.ticket, actual.prepared.source.parents[0].ID, o); err != nil {
		return err
	}
	if err = r.dispatch.resources.ReleaseTx(owned, tx, b, actual.proof); err != nil {
		return err
	}
	if _, err = checkInputTx(owned, tx, actual.grant, b, actual.effect); err != nil {
		return err
	}
	if err = r.dispatch.provenance.CheckTx(owned, tx, actual.prepared.source, actual.proof.LeaseID); err != nil {
		return err
	}
	return tx.Commit(owned)
}

func (r *Runtime) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	fail := func() (*runtimev1.ExecuteResponse, error) {
		call := req.GetToolCall()
		return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "effect_unknown", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: "workspace dispatch denied or its outcome is unknown", Retryable: false}}}, nil
	}
	if r == nil || req == nil {
		return fail()
	}
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	tickets := metadata.ValueFromIncomingContext(ctx, TicketHeader)
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+r.token)) != 1 || len(tickets) != 1 {
		return fail()
	}
	owned, d, a, stop, err := r.dispatch.begin(ctx, req, tickets[0])
	if err != nil {
		return fail()
	}
	defer stop()
	ref, proof := d.prepared.source.ref, privateProof(d)
	var output any
	switch d.prepared.tool {
	case CreateTool:
		if err = r.files.Create(owned, ref, proof); err != nil {
			return fail()
		}
		output = struct {
			WorkspaceID string `json:"workspace_id"`
		}{ref.ID}
	case WriteTool:
		s, writeErr := r.files.Write(owned, ref, proof, a.Expected, a.Content)
		if writeErr != nil {
			return fail()
		}
		output = struct {
			WorkspaceID string             `json:"workspace_id"`
			Snapshot    workspace.Snapshot `json:"snapshot"`
		}{ref.ID, s}
	case ReadTool:
		f, readErr := r.files.Read(owned, ref, proof, a.Path)
		if readErr != nil || len(f.Content) > MaxCodeBytes || !utf8.Valid(f.Content) || sanitize.Text(string(f.Content)) != string(f.Content) {
			return fail()
		}
		if err = r.finishRead(owned, d, f); err != nil {
			return fail()
		}
		output = struct {
			WorkspaceID string             `json:"workspace_id"`
			Snapshot    workspace.Snapshot `json:"snapshot"`
			Content     string             `json:"content"`
		}{ref.ID, f.Snapshot, string(f.Content)}
	default:
		return fail()
	}
	if owned.Err() != nil {
		return fail()
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return fail()
	}
	call := req.GetToolCall()
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}}, nil
}

// Privacy cleanup is an independent bounded system job. It erases only durable
// Catalog deletion entries and cannot renew a worker, spend a tool or reconcile.
func (r *Runtime) RunPrivacyCleanup(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		bounded, stop := context.WithTimeout(ctx, 5*time.Second)
		_, _ = r.dispatch.provenance.PurgeDeleted(bounded, r.purge)
		stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
