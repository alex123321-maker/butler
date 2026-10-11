package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/resources"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

type App struct {
	postgres *postgresstore.Store
	runtime  *workspaceexec.Runtime
	grpc     *grpc.Server
	listener net.Listener
	http     *http.Server
	socket   string
}
type denyReconciliation struct{}

func (denyReconciliation) CheckReconciliationTx(context.Context, pgx.Tx, resources.Decision) error {
	return resources.ErrDenied
}

func New(ctx context.Context) (*App, error) {
	cfg, _, err := config.LoadToolWorkspaceFromEnv()
	if err != nil {
		return nil, err
	}
	pool, err := postgresstore.Open(ctx, postgresstore.ConfigFromShared(cfg.Postgres), nil)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			pool.Close()
		}
	}()
	token, err := actionauth.ReadServiceToken(cfg.TokenFile)
	if err != nil {
		return nil, err
	}
	provenance, err := workspaceexec.NewProvenance(pool.Pool(), catalog.New(pool.Pool(), catalog.NewFileManifest(cfg.DeletionManifest)))
	if err != nil {
		return nil, err
	}
	leaseStore, err := resources.New(resources.WorkerGuard{}, denyReconciliation{})
	if err != nil {
		return nil, err
	}
	dispatch, err := workspaceexec.NewDispatchStore(pool.Pool(), leaseStore, budget.NewController(pool.Pool(), budget.DefaultPolicy()), provenance, workspaceexec.Profile{ID: cfg.ProfileID, Target: cfg.Shared.GRPCAddr})
	if err != nil {
		return nil, err
	}
	r, err := workspaceexec.NewRuntime(ctx, dispatch, cfg.Root, token)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			_ = r.Close()
		}
	}()
	h, err := workspaceexec.NewHealth(r)
	if err != nil {
		return nil, err
	}
	listener, socket, err := listen(cfg.Shared.GRPCAddr)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(512*1024), grpc.MaxSendMsgSize(512*1024))
	runtimev1.RegisterToolRuntimeServiceServer(server, r)
	healthv1.RegisterHealthServer(server, h)
	success = true
	return &App{pool, r, server, listener, &http.Server{Addr: cfg.Shared.HTTPAddr, Handler: health.New("tool-workspace").Handler(), ReadHeaderTimeout: 5 * time.Second}, socket}, nil
}

// Only a fixed private operator socket may be removed after a crash. An active,
// non-socket, inaccessible or changed endpoint is never replaced.
func listen(target string) (net.Listener, string, error) {
	if target != workspaceexec.UnixTarget {
		l, err := net.Listen("tcp", target)
		return l, "", err
	}
	socket := workspaceexec.UnixSocket
	parent := filepath.Dir(socket)
	dir, err := os.Lstat(parent)
	if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 || dir.Mode().Perm()&0077 != 0 {
		return nil, "", workspaceexec.ErrDenied
	}
	if info, e := os.Lstat(socket); e == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, "", workspaceexec.ErrDenied
		}
		conn, dialErr := net.DialTimeout("unix", socket, 150*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, "", workspaceexec.ErrDenied
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, "", workspaceexec.ErrDenied
		}
		current, e := os.Lstat(socket)
		if e != nil || !os.SameFile(info, current) {
			return nil, "", workspaceexec.ErrDenied
		}
		if e = os.Remove(socket); e != nil {
			return nil, "", e
		}
	} else if !os.IsNotExist(e) {
		return nil, "", workspaceexec.ErrDenied
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, "", err
	}
	current, e := os.Lstat(parent)
	if e != nil || !os.SameFile(dir, current) || os.Chmod(socket, 0600) != nil {
		_ = l.Close()
		return nil, "", workspaceexec.ErrDenied
	}
	return l, socket, nil
}
func (a *App) Run(ctx context.Context) error {
	cleanupCtx, stop := context.WithCancel(ctx)
	failures := make(chan error, 2)
	go func() { failures <- a.grpc.Serve(a.listener) }()
	go func() { failures <- a.http.ListenAndServe() }()
	cleanupDone := make(chan struct{})
	go func() { defer close(cleanupDone); _ = a.runtime.RunPrivacyCleanup(cleanupCtx) }()
	var err error
	select {
	case err = <-failures:
	case <-ctx.Done():
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.http.Shutdown(shutdown)
	grpcDone := make(chan struct{})
	go func() { a.grpc.GracefulStop(); close(grpcDone) }()
	select {
	case <-grpcDone:
	case <-shutdown.Done():
		a.grpc.Stop()
	}
	_ = a.listener.Close()
	select {
	case <-cleanupDone:
	case <-shutdown.Done():
	}
	_ = a.runtime.Close()
	a.postgres.Close()
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
