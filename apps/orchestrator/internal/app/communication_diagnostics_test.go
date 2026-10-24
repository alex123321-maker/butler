package app

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/communicationcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
)

type diagnosticBroker struct {
	toolbrokerv1.UnimplementedToolBrokerServiceServer
}

func (diagnosticBroker) GetToolContract(_ context.Context, r *toolbrokerv1.GetToolContractRequest) (*toolbrokerv1.GetToolContractResponse, error) {
	c := communicationcontrol.Contract(r.GetToolName())
	if r.GetToolName() == "communication.report" {
		c.RuntimeTarget = "broker://unexpected-target"
	}
	return &toolbrokerv1.GetToolContractResponse{Contract: c}, nil
}

func TestCommunicationDiagnosticChecksActualBrokerContract(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	toolbrokerv1.RegisterToolBrokerServiceServer(srv, diagnosticBroker{})
	go srv.Serve(l)
	t.Cleanup(srv.Stop)
	client, err := tools.Dial(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	probe := communicationToolProbe(client)
	out, err := probe(ctx)
	if err != nil || !out["communication.message"] || out["communication.report"] {
		t.Fatal("flags replaced actual contract proof", out, err)
	}
	if communicationToolProbe(nil) != nil {
		t.Fatal("missing Broker became ready")
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err = probe(canceled); err == nil {
		t.Fatal("unobserved Broker called ready")
	}
}
