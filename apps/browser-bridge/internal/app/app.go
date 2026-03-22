package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/bridge"
	"github.com/butler/butler/apps/browser-bridge/internal/client"
	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
)

type App struct {
	config config.BrowserBridgeConfig
	log    *slog.Logger
	client *client.Client
	bridge *bridge.Dispatcher
	stdin  io.Reader
	stdout io.Writer

	httpServer *http.Server
	writeMu    sync.Mutex
}

func New(_ context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) (*App, error) {
	cfg, _, err := config.LoadBrowserBridgeFromEnv()
	if err != nil {
		return nil, err
	}
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	log := logger.New(logger.Options{
		Service:   cfg.Shared.ServiceName,
		Component: "browser-bridge",
		Level:     parseLogLevel(cfg.Shared.LogLevel),
		Writer:    stderr,
	})
	app := &App{
		config: cfg,
		log:    log,
		client: client.New(cfg.OrchestratorBaseURL, time.Duration(cfg.RequestTimeoutSeconds)*time.Second, nil),
		stdin:  stdin,
		stdout: stdout,
	}
	app.bridge = bridge.NewDispatcher(app.writeNativeRequest)

	mux := http.NewServeMux()
	mux.Handle("/health", health.New(cfg.Shared.ServiceName).Handler())
	mux.HandleFunc("/api/v1/actions/dispatch", app.handleDispatchAction)
	app.httpServer = &http.Server{
		Addr:              cfg.ControlAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.log.Info("starting browser bridge control server", slog.String("addr", a.config.ControlAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.httpServer.Shutdown(shutdownCtx)
	}()

	for {
		select {
		case err := <-errCh:
			return err
		default:
		}

		message, err := protocol.ReadMessage(a.stdin)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		switch payload := message.(type) {
		case protocol.Request:
			response := a.handleRequest(ctx, payload)
			if err := a.writeNativeResponse(response); err != nil {
				return err
			}
		case protocol.Response:
			if !a.bridge.Resolve(payload) {
				a.log.Warn("received uncorrelated native response", slog.String("id", payload.ID))
			}
		}
	}
}

func (a *App) handleRequest(ctx context.Context, request protocol.Request) protocol.Response {
	switch strings.TrimSpace(request.Method) {
	case "ping":
		return protocol.Response{
			ID: request.ID,
			OK: true,
			Result: map[string]any{
				"service": a.config.Shared.ServiceName,
				"status":  "ok",
			},
		}
	case "bind.request":
		var params protocol.BindRequestParams
		if err := decodeParams(request.Params, &params); err != nil {
			return invalidRequest(request.ID, err)
		}
		result, err := a.client.CreateBindRequest(ctx, params)
		if err != nil {
			return mapError(request.ID, err)
		}
		return protocol.Response{ID: request.ID, OK: true, Result: result}
	case "session.get_active":
		var params protocol.SessionGetActiveParams
		if err := decodeParams(request.Params, &params); err != nil {
			return invalidRequest(request.ID, err)
		}
		result, err := a.client.GetActiveSession(ctx, params.SessionKey)
		if err != nil {
			return mapError(request.ID, err)
		}
		return protocol.Response{ID: request.ID, OK: true, Result: result}
	default:
		return protocol.Response{
			ID: request.ID,
			OK: false,
			Error: &protocol.ErrorPayload{
				Code:    "unsupported_method",
				Message: fmt.Sprintf("unsupported method %q", request.Method),
			},
		}
	}
}

func decodeParams(raw any, dest any) error {
	if raw == nil {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

func invalidRequest(id string, err error) protocol.Response {
	return protocol.Response{
		ID: id,
		OK: false,
		Error: &protocol.ErrorPayload{
			Code:    "invalid_request",
			Message: err.Error(),
		},
	}
}

func mapError(id string, err error) protocol.Response {
	code := "upstream_error"
	message := err.Error()
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 404:
			code = "not_found"
		case 409:
			code = "conflict"
		case 400:
			code = "invalid_request"
		}
		message = apiErr.Message
	}
	return protocol.Response{
		ID: id,
		OK: false,
		Error: &protocol.ErrorPayload{
			Code:    code,
			Message: message,
		},
	}
}

func (a *App) handleDispatchAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var request protocol.ActionDispatchParams
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body", "code": "invalid_request"})
		return
	}
	if strings.TrimSpace(request.SingleTabSessionID) == "" || strings.TrimSpace(request.BoundTabRef) == "" || strings.TrimSpace(request.ActionType) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "single_tab_session_id, bound_tab_ref, and action_type are required", "code": "invalid_request"})
		return
	}

	result, err := a.bridge.DispatchAction(r.Context(), request)
	if err != nil {
		status := http.StatusInternalServerError
		code := "dispatch_failed"
		message := err.Error()

		if errors.Is(err, bridge.ErrNoNativeClient) {
			status = http.StatusServiceUnavailable
			code = "host_unavailable"
		}
		var dispatchErr *bridge.DispatchError
		if errors.As(err, &dispatchErr) {
			code = dispatchErr.Code
			message = dispatchErr.Message
			switch dispatchErr.Code {
			case "tab_closed":
				status = http.StatusConflict
			case "selector_not_found", "action_not_allowed":
				status = http.StatusBadRequest
			case "host_unavailable":
				status = http.StatusServiceUnavailable
			}
		}
		writeJSON(w, status, map[string]string{"error": message, "code": code})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"action": result})
}

func (a *App) writeNativeRequest(request protocol.Request) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return protocol.WriteRequest(a.stdout, request)
}

func (a *App) writeNativeResponse(response protocol.Response) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return protocol.WriteResponse(a.stdout, response)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseLogLevel(value string) slog.Leveler {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
