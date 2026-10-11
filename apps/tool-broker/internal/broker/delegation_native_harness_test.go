package broker

import (
	"context"
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
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

// The native experiment has a longer wall-clock bound than the zero-inference
// admission suite. It still uses the real private Broker methods, task catalog,
// current HTTP authority, PG journal and source guard; no runtime/mock route.
func TestNativeDelegationBrokerHarness(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_NATIVE_BROKER_HARNESS") != "1" {
		t.Skip("private native experiment Broker only")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_DELEGATION_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(cfg.ConnConfig.RuntimeParams["search_path"], "memory_") {
		t.Fatal("isolated schema/database required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("private Broker storage unavailable")
	}
	defer pool.Close()
	token, err := actionauth.ReadServiceToken(os.Getenv("BUTLER_DELEGATION_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private Broker proof unavailable")
	}
	reader, err := actionauth.NewHTTPContextReader(os.Getenv("BUTLER_DELEGATION_CONTEXT_URL"), token)
	if err != nil {
		t.Fatal("private authority endpoint unavailable")
	}
	admission, err := delegation.NewAdmissionClient(os.Getenv("BUTLER_DELEGATION_ADMISSION_URL"), token)
	if err != nil {
		t.Fatal("private admission endpoint unavailable")
	}
	regPath := filepath.Join(t.TempDir(), "tools.json")
	if os.WriteFile(regPath, []byte(`{"tools":[]}`), 0600) != nil {
		t.Fatal("fixture registry unavailable")
	}
	reg, err := registry.Load(regPath, "")
	if err != nil || reg.RegisterOwned([]*toolbrokerv1.ToolContract{delegation.AdmissionContract()}) != nil {
		t.Fatal("fixed reviewed contract unavailable")
	}
	policy, err := budget.LoadPolicy()
	if err != nil || policy.Day.ModelTurns != 12 || policy.Mandate.ModelTurns != 12 || policy.Task.ModelTurns != 6 {
		t.Fatal("finite experiment policy required")
	}
	controller := budget.NewController(pool, policy)
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	journal := actionauth.NewPostgresStore(pool)
	journal.SetBudgets(controller)
	sources := catalog.New(pool, catalog.NewFileManifest(os.Getenv("BUTLER_DELEGATION_MANIFEST_FILE")))
	journal.SetRunDataGuard(sources.CheckRunSourcesTx)
	server := NewServer(reg, &DelegationExecutor{Admissions: admission}, nil, nil)
	server.SetActionAuthorizer(actionauth.NewService(journal, reader, nil))
	server.SetBudgets(controller)
	server.SetScopedCatalog(&TaskCatalog{Registry: reg, Contexts: reader, Mandates: journal})
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_DELEGATION_GRPC_ADDRESS"))
	if err != nil {
		t.Fatal("private Broker listener unavailable")
	}
	defer listener.Close()
	g := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(token)))
	toolbrokerv1.RegisterToolBrokerServiceServer(g, server)
	defer g.Stop()
	served := make(chan error, 1)
	go func() { served <- g.Serve(listener) }()
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt)
	defer signal.Stop(stopped)
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("private Broker experiment deadline")
	case <-served:
		t.Fatal("private Broker stopped unexpectedly")
	}
}
