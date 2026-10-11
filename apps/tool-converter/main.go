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

	"github.com/butler/butler/apps/tool-converter/internal/runtime"
	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/health"
	"google.golang.org/grpc"
)

func main() {
	if e := run(); e != nil {
		slog.Error("bounded converter runtime stopped", slog.String("reason", "runtime configuration or service failure"))
		os.Exit(1)
	}
}
func run() error {
	profile, e := generatedtools.DecodeProfile([]byte(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")))
	if e != nil {
		return e
	}
	token, e := actionauth.ReadServiceToken(os.Getenv("BUTLER_CONVERTER_RUNTIME_TOKEN_FILE"))
	if e != nil {
		return e
	}
	grpcAddr := os.Getenv("BUTLER_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":9090"
	}
	httpAddr := os.Getenv("BUTLER_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	tempDir := os.Getenv("BUTLER_CONVERTER_TEMP_DIR")
	if tempDir == "" {
		tempDir = "/tmp"
	}
	listener, e := net.Listen("tcp", grpcAddr)
	if e != nil {
		return e
	}
	defer listener.Close()
	server := grpc.NewServer(grpc.UnaryInterceptor(generatedtools.RuntimeAuthentication(token)), grpc.MaxRecvMsgSize(192*1024), grpc.MaxSendMsgSize(512*1024))
	converterv1.RegisterConverterRuntimeServiceServer(server, runtime.NewServer(tempDir, profile))
	healthServer := &http.Server{Addr: httpAddr, Handler: health.New("tool-converter").Handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 2)
	go func() { failures <- server.Serve(listener) }()
	go func() { failures <- healthServer.ListenAndServe() }()
	select {
	case e = <-failures:
	case <-ctx.Done():
		e = nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = healthServer.Shutdown(shutdown)
	server.Stop()
	return e
}
