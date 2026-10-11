package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/bridge"
	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/singletabcontrol"
	"google.golang.org/grpc"
)

// RunGuarded is an additive opt-in native host. The listener is operator-owned;
// it exposes only the authenticated, claimed runtime, with NO legacy dispatch
// or bind HTTP route. The original native companion is not a secure substitute.
func RunGuarded(ctx context.Context, stdin io.Reader, stdout io.Writer, listener net.Listener, authority resourceexec.RuntimeAuthority, brokerToken string) error {
	return RunGuardedWithMetadata(ctx, stdin, stdout, listener, nil, authority, brokerToken)
}

// Separate fixed authenticated metadata only; no raw action/bind endpoint.
func RunGuardedWithMetadata(ctx context.Context, stdin io.Reader, stdout io.Writer, listener, metadata net.Listener, authority resourceexec.RuntimeAuthority, brokerToken string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var writeMu sync.Mutex
	dispatcher := bridge.NewDispatcher(func(r protocol.Request) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return protocol.WriteRequest(stdout, r)
	})
	if metadata != nil {
		h, e := singletabcontrol.NewMetadataHandler(bridge.NewGuardedMetadata(dispatcher), brokerToken)
		if e != nil {
			return e
		}
		s := &http.Server{Handler: h, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
		defer s.Close()
		go func() { _ = s.Serve(metadata) }()
		go func() { <-ctx.Done(); _ = s.Close() }()
	}
	runtime, e := resourceexec.NewRuntime(bridge.NewGuardedBackend(dispatcher), authority, brokerToken)
	if e != nil {
		return e
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(65536), grpc.MaxSendMsgSize(65536))
	runtimev1.RegisterToolRuntimeServiceServer(server, runtime)
	defer server.Stop()
	go func() { _ = server.Serve(listener) }()
	go func() {
		<-ctx.Done()
		server.Stop()
		if closer, ok := stdin.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	for {
		message, e := protocol.ReadGuardedMessage(stdin)
		if e != nil {
			return e
		}
		switch value := message.(type) {
		case protocol.Response:
			dispatcher.Resolve(value)
		case protocol.Request:
			// The extension can ping but cannot grant/queue arbitrary host operations.
			response := protocol.Response{ID: value.ID, OK: value.Method == "ping"}
			if !response.OK {
				response.Error = &protocol.ErrorPayload{Code: "guarded_only", Message: "Guarded host accepts only claimed runtime dispatch"}
			}
			writeMu.Lock()
			e = protocol.WriteResponse(stdout, response)
			writeMu.Unlock()
			if e != nil {
				return e
			}
		}
	}
}
