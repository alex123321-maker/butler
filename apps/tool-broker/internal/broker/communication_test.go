package broker

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"strings"
	"testing"
)

type communicationService struct {
	calls []communicationcontrol.Envelope
	lost  bool
}

func (s *communicationService) ExecuteDispatched(_ context.Context, e communicationcontrol.Envelope) (communicationcontrol.Receipt, error) {
	s.calls = append(s.calls, e)
	if s.lost && !e.Lookup {
		return communicationcontrol.Receipt{}, errors.New("lost ACK")
	}
	return communicationcontrol.Receipt{Status: "accepted", PublicationRef: "publication:original", Delivery: []communicationcontrol.Delivery{}}, nil
}
func TestCommunicationRouteAndLostACKLookup(t *testing.T) {
	service := &communicationService{lost: true}
	route := &CommunicationExecutor{Communication: service}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "communication.message", ArgsJson: `{"schema_version":1,"purpose":"result","text":"ready"}`}
	contract := communicationcontrol.Contract(call.ToolName)
	if _, e := route.Execute(context.Background(), call, contract, nil); e == nil || len(service.calls) != 0 {
		t.Fatal("direct route without dispatch")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "original", Fingerprint: strings.Repeat("a", 64)})
	r, e := route.Execute(ctx, call, contract, nil)
	if e != nil || r.Status != "completed" || len(service.calls) != 2 || service.calls[0].Lookup || !service.calls[1].Lookup || service.calls[0].EffectID != service.calls[1].EffectID {
		t.Fatal("lost ACK republished instead of looking up", e)
	}
}
