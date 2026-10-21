package broker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type delegationReceiptFixture struct {
	calls    int
	envelope delegation.AdmissionEnvelope
}

func (f *delegationReceiptFixture) AdmitDispatched(_ context.Context, e delegation.AdmissionEnvelope) (delegation.AdmissionReceipt, error) {
	f.calls++
	f.envelope = e
	return delegation.AdmissionReceipt{TaskID: "child", InputID: "input", State: "queued", ContractRevision: 1}, nil
}

type delegationNextFixture struct{ calls int }

func (f *delegationNextFixture) Execute(_ context.Context, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	f.calls++
	return &toolbrokerv1.ToolResult{Status: "completed"}, nil
}

func TestDelegationExecutorPrivateDispatchOnly(t *testing.T) {
	a := delegation.AdmissionArgs{ContractSource: delegation.SourceRef{ID: "contract", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}, Sources: []delegation.SourceRef{}, Dependencies: []string{}, Budget: delegation.Limits{ModelTurns: 1}, Deadline: time.Now().Add(time.Minute)}
	raw, _ := json.Marshal(a)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: delegation.AdmissionTool, ArgsJson: string(raw)}
	receipts := &delegationReceiptFixture{}
	next := &delegationNextFixture{}
	route := &DelegationExecutor{Admissions: receipts, Next: next}
	if _, err := route.Execute(context.Background(), call, delegation.AdmissionContract(), nil); err == nil || receipts.calls != 0 {
		t.Fatal("model IDs alone reached admission")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "permanent-effect", Fingerprint: strings.Repeat("b", 64)})
	contract := delegation.AdmissionContract()
	contract.RuntimeTarget = "http://model-chosen-runtime"
	if _, err := route.Execute(ctx, call, contract, nil); err == nil || receipts.calls != 0 {
		t.Fatal("operator registry replaced fixed control contract")
	}
	if _, err := route.Execute(ctx, call, delegation.AdmissionContract(), []credentials.ResolvedSecret{{}}); err == nil || receipts.calls != 0 {
		t.Fatal("raw credential path reached delegation control")
	}
	result, err := route.Execute(ctx, call, delegation.AdmissionContract(), nil)
	if err != nil || result.GetStatus() != "completed" || receipts.calls != 1 || next.calls != 0 || receipts.envelope.EffectID != "permanent-effect" || strings.Contains(result.GetResultJson(), "permanent-effect") {
		t.Fatal(result, err)
	}
	other := &toolbrokerv1.ToolCall{ToolName: "other"}
	if _, err = route.Execute(ctx, other, nil, nil); err != nil || next.calls != 1 {
		t.Fatal("existing runtime route lost", err)
	}
}

// This opt-in harness is a real Broker gRPC process used by the orchestrator
// acceptance fixture. It never loads the production registry, secrets or runtime
// endpoints and requires a disposable *_test database with an isolated schema.
func TestDelegationBrokerHarness(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_HARNESS") != "1" {
		t.Skip("private process harness only")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_DELEGATION_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(cfg.ConnConfig.RuntimeParams["search_path"], "memory_") {
		t.Fatal("isolated schema/database required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	path := os.Getenv("BUTLER_DELEGATION_TOKEN_FILE")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private regular fixture token required")
	}
	token, err := actionauth.ReadServiceToken(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := actionauth.NewHTTPContextReader(os.Getenv("BUTLER_DELEGATION_CONTEXT_URL"), token)
	if err != nil {
		t.Fatal(err)
	}
	var executor RuntimeExecutor
	contract := delegation.AdmissionContract()
	if os.Getenv("BUTLER_COMMUNICATION_HARNESS") == "1" {
		client, err := communicationcontrol.NewClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &CommunicationExecutor{Communication: client}
		contract = communicationcontrol.Contract("communication.message")
	} else if os.Getenv("BUTLER_MEMORY_FACT_HARNESS") == "1" {
		client, err := factcontrol.NewClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &MemoryFactExecutor{Facts: client}
		contract = factcontrol.Contract()
	} else if tool := os.Getenv("BUTLER_MEMORY_READ_HARNESS"); readcontrol.IsTool(tool) {
		client, err := readcontrol.NewClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &MemoryReadExecutor{Reads: client}
		contract = readcontrol.Contract(tool)
	} else if os.Getenv("BUTLER_MEMORY_CANDIDATE_HARNESS") == "1" {
		client, err := candidatecontrol.NewClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &MemoryCandidateExecutor{Candidates: client}
		contract = candidatecontrol.Contract()
	} else if os.Getenv("BUTLER_GUARDED_TAB_REQUEST_HARNESS") == "1" {
		client, err := singletabcontrol.NewRequestClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &GuardedTabRequestExecutor{Requests: client}
		contract = singletabcontrol.RequestContract()
	} else {
		admission, err := delegation.NewAdmissionClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
		if err != nil {
			t.Fatal(err)
		}
		executor = &DelegationExecutor{Admissions: admission}
	}
	regPath := filepath.Join(t.TempDir(), "tools.json")
	if err = os.WriteFile(regPath, []byte(`{"tools":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(regPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = reg.RegisterOwned([]*toolbrokerv1.ToolContract{contract}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("BUTLER_COMMUNICATION_HARNESS") == "1" {
		if err = reg.RegisterOwned([]*toolbrokerv1.ToolContract{communicationcontrol.Contract("communication.widget")}); err != nil {
			t.Fatal(err)
		}
	}
	controller := budget.NewController(pool, budget.DefaultPolicy())
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	journal := actionauth.NewPostgresStore(pool)
	journal.SetBudgets(controller)
	sources := catalog.New(pool, catalog.NewFileManifest(os.Getenv("BUTLER_DELEGATION_MANIFEST_FILE")))
	journal.SetRunDataGuard(sources.CheckRunSourcesTx)
	server := NewServer(reg, executor, nil, nil)
	server.SetScopedCatalog(&TaskCatalog{Registry: reg, Contexts: reader, Mandates: journal})
	server.SetActionAuthorizer(actionauth.NewService(journal, reader, nil))
	server.SetBudgets(controller)
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_DELEGATION_GRPC_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(token)))
	toolbrokerv1.RegisterToolBrokerServiceServer(grpcServer, server)
	defer grpcServer.Stop()
	served := make(chan error, 1)
	go func() { served <- grpcServer.Serve(listener) }()
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt)
	defer signal.Stop(stopped)
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("harness deadline exceeded")
	case err = <-served:
		if err != nil {
			t.Fatal(err)
		}
	}
}
