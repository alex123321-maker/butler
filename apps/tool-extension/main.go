package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/butler/butler/apps/tool-extension/internal/runtime"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/health"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reviewed extension runtime stopped", slog.String("reason", err.Error()))
		os.Exit(1)
	}
}
func run() error {
	profiles, err := extensions.ParseProfiles(os.Getenv("BUTLER_EXTENSION_PROFILES_JSON"))
	if err != nil {
		return err
	}
	token, err := actionauth.ReadServiceToken(os.Getenv("BUTLER_EXTENSION_RUNTIME_TOKEN_FILE"))
	if err != nil {
		return err
	}
	grpcAddr := os.Getenv("BUTLER_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":9090"
	}
	httpAddr := os.Getenv("BUTLER_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := grpc.NewServer(grpc.UnaryInterceptor(extensions.RuntimeAuthentication(token)))
	runtimev1.RegisterToolRuntimeServiceServer(server, runtime.NewServer(profiles))
	healthServer := &http.Server{Addr: httpAddr, Handler: health.New("tool-extension").Handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 2)
	go func() { failures <- server.Serve(listener) }()
	go func() { failures <- healthServer.ListenAndServe() }()
	select {
	case err = <-failures:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = healthServer.Shutdown(shutdown)
	server.Stop()
	return err
}
