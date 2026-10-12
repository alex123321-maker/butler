package app

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Explicit test composition, never a production transport fallback. Prior
// immutable evidence supplies actual High parent/width1 generation. Only the
// two current width2 children can enter the native provider in this remainder.
type highRemainderProvider struct {
	*nativeScriptedObservationProvider
	pool   *pgxpool.Pool
	corpus nativeResearchCorpus
	native transport.ModelProvider
	mu     sync.Mutex
	modes  map[string]string
}

func newHighRemainderProvider(pool *pgxpool.Pool, corpus nativeResearchCorpus, args []delegation.AdmissionArgs, native transport.ModelProvider) (*highRemainderProvider, error) {
	if pool == nil || native == nil || len(args) != 2 || len(corpus.Sources) != 2 {
		return nil, errNativeFixtureDenied
	}
	for _, a := range args {
		if _, err := delegation.DecodeAdmissionArgs(string(nativeJSON(a))); err != nil || len(a.Sources) != 1 {
			return nil, errNativeFixtureDenied
		}
	}
	return &highRemainderProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: args, receipts: map[string][]string{}}, pool: pool, corpus: corpus, native: native, modes: map[string]string{}}, nil
}

func (p *highRemainderProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if !highRemainderDelegationWindow() || nativeContextObserver(p.pool, p.corpus)(ctx, req) != nil {
		return nil, errNativeFixtureDenied
	}
	var ownSession, parentSession, owner string
	var raw []byte
	if p.pool.QueryRow(ctx, `SELECT t.session_key,COALESCE(parent.session_key,''),t.owner_id,d.contract_json FROM runs r JOIN tasks t ON t.task_id=r.task_id LEFT JOIN tasks parent ON parent.task_id=t.parent_task_id LEFT JOIN task_delegations d ON d.task_id=t.task_id WHERE r.run_id=$1`, req.Context.RunID).Scan(&ownSession, &parentSession, &owner, &raw) != nil || ownSession != req.Context.SessionKey || owner != nativeFixtureOwner {
		return nil, errNativeFixtureDenied
	}
	mode := "scripted_parent"
	if parentSession == "" {
		if len(raw) != 0 || ownSession != "native-root-width-1" && ownSession != "native-root-width-2" {
			return nil, errNativeFixtureDenied
		}
	} else {
		var contract delegation.Contract
		if json.Unmarshal(raw, &contract) != nil || len(contract.Sources) != 1 || len(contract.Permissions) != 0 || len(req.ToolDefinitions) != 0 {
			return nil, errNativeFixtureDenied
		}
		selected := -1
		for i, arg := range p.args {
			if contract.ContractSource == arg.ContractSource && contract.Sources[0] == arg.Sources[0] {
				selected = i
			}
		}
		if selected < 0 || parentSession != "native-root-width-1" && parentSession != "native-root-width-2" {
			return nil, errNativeFixtureDenied
		}
		if parentSession == "native-root-width-2" {
			mode = "native_child"
		} else {
			mode = "scripted_child"
		}
		p.mu.Lock()
		if _, exists := p.modes[req.Context.RunID]; exists {
			p.mu.Unlock()
			return nil, errNativeFixtureDenied
		}
		p.modes[req.Context.RunID] = mode
		p.mu.Unlock()
		if mode == "native_child" {
			return p.native.StartRun(ctx, req)
		}
		ref := contract.Sources[0]
		observation := delegation.NumericObservation{SourceID: ref.ID, SourceRevision: ref.Revision, SourceHash: ref.Hash, ObservedValue: p.corpus.Sources[selected].ExpectedValue, ObservedDate: p.corpus.Sources[selected].ExpectedDate, Uncertainty: "source_only"}
		return nativeScriptedStream(transport.NewAssistantFinalEvent(req.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(nativeJSON(observation)), FinishReason: "stop"}), transport.NewTerminalEvent(req.Context.RunID, transport.EventTypeRunCompleted, p.Name())), nil
	}
	p.mu.Lock()
	if _, exists := p.modes[req.Context.RunID]; exists {
		p.mu.Unlock()
		return nil, errNativeFixtureDenied
	}
	p.modes[req.Context.RunID] = mode
	p.mu.Unlock()
	return p.nativeScriptedObservationProvider.StartRun(ctx, req)
}

func (p *highRemainderProvider) mode(run string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.modes[run]
}
func (p *highRemainderProvider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if p.mode(req.RunID) != "native_child" {
		return nil, errNativeFixtureDenied
	}
	return p.native.ContinueRun(ctx, req)
}
func (p *highRemainderProvider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p.mode(req.RunID) != "scripted_parent" {
		return nil, errNativeFixtureDenied
	}
	return p.nativeScriptedObservationProvider.SubmitToolResult(ctx, req)
}
func (p *highRemainderProvider) CancelRun(ctx context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if p.mode(req.RunID) == "native_child" {
		return p.native.CancelRun(ctx, req)
	}
	return nil, nil
}

func TestHighRemainderDelegationActualAcceptanceIntegration(t *testing.T) {
	if !highRemainderDelegationWindow() || os.Getenv("BUTLER_DELEGATION_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("separate two-call remainder opt-in required")
	}
	runNativeDelegationActualScenario(t)
}
