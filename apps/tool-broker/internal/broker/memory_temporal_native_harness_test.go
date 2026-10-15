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
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

// Trusted isolated composition of the real Broker Server. Production's false
// qualification constant is unchanged; no request/environment flag enables it.
// This process has no cloud transport, runtime, secrets or arbitrary HTTP tool.
func TestTemporalNativeBrokerHarness(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_BROKER_HARNESS") != "1" {
		t.Skip("explicit isolated temporal Broker composition required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEMPORAL_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(cfg.ConnConfig.RuntimeParams["search_path"], "memory_") {
		t.Fatal("isolated test schema required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated PG unavailable")
	}
	defer pool.Close()
	token, err := actionauth.ReadServiceToken(os.Getenv("BUTLER_TEMPORAL_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private token unavailable")
	}
	policy, err := budget.ParsePolicy(os.Getenv("BUTLER_EXECUTION_BUDGET_POLICY_JSON"))
	if err != nil {
		t.Fatal("shared finite policy unavailable")
	}
	contexts, err := actionauth.NewHTTPContextReader(os.Getenv("BUTLER_TEMPORAL_CONTEXT_URL"), token)
	if err != nil {
		t.Fatal("private context boundary unavailable")
	}
	facts, err := factcontrol.NewClient(os.Getenv("BUTLER_TEMPORAL_FACTS_URL"), token)
	if err != nil {
		t.Fatal("private facts boundary unavailable")
	}
	reads, err := readcontrol.NewClient(os.Getenv("BUTLER_TEMPORAL_READ_URL"), token)
	if err != nil {
		t.Fatal("private read boundary unavailable")
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if os.WriteFile(path, []byte(`{"tools":[]}`), 0600) != nil {
		t.Fatal("private registry unavailable")
	}
	reg, err := registry.Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = reg.RegisterOwned([]*toolbrokerv1.ToolContract{factcontrol.Contract(), readcontrol.Contract(readcontrol.Open)}); err != nil {
		t.Fatal(err)
	}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	journal := actionauth.NewPostgresStore(pool)
	journal.SetBudgets(b)
	cat := catalog.New(pool, catalog.NewFileManifest(os.Getenv("BUTLER_TEMPORAL_MANIFEST_FILE")))
	journal.SetRunDataGuard(cat.CheckRunSourcesTx)
	server := NewServer(reg, &MemoryFactExecutor{Facts: facts, Next: &MemoryReadExecutor{Reads: reads}}, nil, nil)
	server.SetScopedCatalog(&TaskCatalog{Registry: reg, Contexts: contexts, Mandates: journal})
	server.SetActionAuthorizer(actionauth.NewService(journal, contexts, nil))
	server.SetBudgets(b)
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_TEMPORAL_GRPC_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	g := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(token)))
	toolbrokerv1.RegisterToolBrokerServiceServer(g, server)
	defer g.Stop()
	done := make(chan error, 1)
	go func() { done <- g.Serve(listener) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	defer signal.Stop(stop)
	select {
	case <-stop:
	case <-ctx.Done():
		t.Fatal("bounded Broker harness expired")
	case err := <-done:
		if err != nil {
			t.Fatal("Broker process failed")
		}
	}
}
