// Package resourceexec joins existing Broker action/budget admission to finite
// resource leases. Its tickets are private metadata, never model arguments.
package resourceexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resources"
	"google.golang.org/grpc/metadata"
)

var ErrDenied = errors.New("current guarded resource dispatch is required")

// Profile is immutable operator composition: a stable executor identity,
// private target and exact deployed contract hashes. It is never decoded from
// model/tool/config override arguments. Changing credentials does not partition
// the resource and let another writer enter the same actual browser context.
type Profile struct {
	ID             string
	RuntimeTarget  string
	ContractHashes map[string]string
}

type selection struct {
	profile, target, tool, contract, args, resource, authorization string
	single                                                         *singleTabSelection
}
type selectionKey struct{}
type ticketKey struct{}

const ticketHeader = "butler-resource-ticket"

func Protected(tool string) bool {
	return strings.HasPrefix(tool, "browser.") || strings.HasPrefix(tool, "single_tab.")
}

func WithTicket(ctx context.Context, ticket string) context.Context {
	return context.WithValue(ctx, ticketKey{}, ticket)
}

// OutgoingContext is called only by the private runtime router; no protobuf
// tool args, result, approval or model-visible execution context gains a ticket.
func OutgoingContext(ctx context.Context) context.Context {
	if ticket, ok := ctx.Value(ticketKey{}).(string); ok && validTicket(ticket) {
		return metadata.AppendToOutgoingContext(ctx, ticketHeader, ticket)
	}
	return ctx
}
func validTicket(v string) bool { return len(v) == 48 && isHex(v) }
func isHex(v string) bool       { _, err := hex.DecodeString(v); return err == nil }
func hash(v string) string      { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:]) }

func browserSelection(p Profile, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, g actionauth.Grant) (selection, error) {
	if call == nil || contract == nil || call.GetToolName() != contract.GetToolName() || call.GetRunId() != g.Binding.RunID || call.GetToolCallId() != g.Binding.ToolCallID || call.GetToolName() != g.Binding.ToolName || contract.GetRuntimeTarget() != p.RuntimeTarget {
		return selection{}, ErrDenied
	}
	ch, err := actionauth.ContractHash(contract)
	if err != nil || ch != g.Binding.ContractHash || p.ContractHashes[call.GetToolName()] != ch {
		return selection{}, ErrDenied
	}
	ah, err := actionauth.ArgumentsHash(call.GetArgsJson())
	if err != nil || ah != g.Binding.ArgsHash {
		return selection{}, ErrDenied
	}
	var args struct {
		SessionID string `json:"browser_session_id"`
	}
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil {
		return selection{}, ErrDenied
	}
	identity := args.SessionID
	switch call.GetToolName() {
	case "browser.open", "browser.read", "browser.open_pages":
		if identity != "" {
			return selection{}, ErrDenied
		}
		identity = "new:" + g.Binding.ActionID
	case "browser.pages", "browser.page_snapshot", "browser.page_click", "browser.page_fill", "browser.page_type", "browser.page_close", "browser.freeze_snapshot", "browser.close", "browser.navigate", "browser.snapshot", "browser.extract_text", "browser.wait_for", "browser.click", "browser.fill", "browser.type", "browser.prepare_fill", "browser.prepare_type", "browser.set_cookie", "browser.restore_storage_state":
		if len(identity) != 32 || !isHex(identity) {
			return selection{}, ErrDenied
		}
	default:
		return selection{}, ErrDenied
	}
	return selection{profile: p.ID, target: p.RuntimeTarget, tool: call.GetToolName(), contract: ch, args: ah, resource: "browser-tab:" + hash(p.ID+"\x00"+identity), authorization: g.ID}, nil
}

// RuntimeSpec is private authority metadata. Backend runtimes still validate
// actual session ownership, origin, credential scope and page observation CAS.
type RuntimeSpec struct {
	Lane        string            `json:"lane"`
	Fence       int64             `json:"fence"`
	Deadline    time.Time         `json:"deadline"`
	SingleTab   *SingleTabSession `json:"single_tab,omitempty"`
	Observation string            `json:"observation,omitempty"`
	DispatchID  string            `json:"dispatch_id,omitempty"`
}
type RuntimeRequest struct {
	Ticket       string `json:"ticket"`
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	ActionID     string `json:"action_id"`
	Fingerprint  string `json:"fingerprint"`
	ContractHash string `json:"contract_hash"`
	ArgsHash     string `json:"args_hash"`
}
type Receipt struct {
	RuntimeRequest
	Completed        bool      `json:"completed"`
	Digest           string    `json:"digest"`
	SessionID        string    `json:"session_id,omitempty"`
	SessionExpiresAt time.Time `json:"session_expires_at,omitempty"`
}
type RuntimeAuthority interface {
	Inspect(context.Context, RuntimeRequest) (RuntimeSpec, error)
	Begin(context.Context, RuntimeRequest) (RuntimeSpec, error)
	Finish(context.Context, Receipt) error
}

func runtimeRequest(req *runtimev1.ExecuteRequest, ticket string) (RuntimeRequest, error) {
	call, c, ec := req.GetToolCall(), req.GetContract(), req.GetContext()
	if !validTicket(ticket) || call == nil || c == nil || ec == nil || call.GetRunId() != ec.GetRunId() || call.GetToolCallId() != ec.GetToolCallId() || call.GetToolName() != ec.GetToolName() || ec.GetRuntimeTarget() != c.GetRuntimeTarget() || ec.GetActionId() == "" || ec.GetActionFingerprint() == "" {
		return RuntimeRequest{}, ErrDenied
	}
	ch, e := actionauth.ContractHash(c)
	if e != nil {
		return RuntimeRequest{}, ErrDenied
	}
	ah, e := actionauth.ArgumentsHash(call.GetArgsJson())
	if e != nil {
		return RuntimeRequest{}, ErrDenied
	}
	return RuntimeRequest{ticket, call.GetRunId(), call.GetToolCallId(), call.GetToolName(), ec.GetActionId(), ec.GetActionFingerprint(), ch, ah}, nil
}

type dispatch struct {
	ticket, authorization, action, effect, profile, tool, contract, args, phase string
	binding                                                                     resources.Binding
	proof                                                                       resources.Proof
	ownerLease                                                                  string
	single                                                                      *singleTabSelection
}
