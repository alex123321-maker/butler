package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/butler/butler/apps/tool-converter/internal/runtime"
	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/security/workspace"
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
	// A declared dependency digest must identify this deployed executable.
	// Metadata alone never proves arbitrary supplied code or model weights.
	executable, e := os.Executable()
	if e != nil {
		return generatedtools.ErrAccess
	}
	binary, e := workspace.ReadRegular(filepath.Dir(executable), filepath.Base(executable), 64*1024*1024)
	if e != nil || generatedtools.HashBytes(binary) != profile.Dependency.SHA256 {
		return generatedtools.ErrAccess
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
	network, address := "tcp", grpcAddr
	if socket := os.Getenv("BUTLER_CONVERTER_UNIX_SOCKET"); socket != "" {
		if socket != generatedtools.UnixSocket {
			return generatedtools.ErrAccess
		}
		// The socket directory is a dedicated volume owned by the unprivileged
		// runtime. Packages never choose paths. A restart removes only this socket.
		if info, statErr := os.Lstat(socket); statErr == nil {
			if info.Mode()&os.ModeSocket == 0 {
				return generatedtools.ErrAccess
			}
			if e = os.Remove(socket); e != nil {
				return e
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		network, address = "unix", socket
	}
	listener, e := net.Listen(network, address)
	if e != nil {
		return e
	}
	defer listener.Close()
	if network == "unix" {
		if e = os.Chmod(address, 0600); e != nil {
			return e
		}
		defer os.Remove(address)
	}
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
