package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/jsonstrict"
)

type runtimeRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type runtimeReply struct {
	Phase  string  `json:"phase"`
	Result *Result `json:"result,omitempty"`
	Error  string  `json:"error,omitempty"`
}

func RunRemoteAntigravity(ctx context.Context, socket, model, prompt string, before func(context.Context) error) (Result, error) {
	if !filepath.IsAbs(socket) || strings.ContainsRune(socket, 0) || !strings.HasPrefix(model, "gemini-") || len(prompt) == 0 || len(prompt) > 2<<20 {
		return Result{}, ErrProcess
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return Result{}, ErrProcess
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if conn.SetDeadline(deadline) != nil {
		return Result{}, ErrProcess
	}
	if err := writeRuntimeFrame(conn, runtimeRequest{Model: model, Prompt: prompt}, 4<<20); err != nil {
		return Result{}, err
	}
	reader := bufio.NewReaderSize(conn, 1<<20)
	var ready runtimeReply
	if err := readRuntimeFrame(reader, &ready); err != nil {
		return Result{}, err
	}
	if ready.Error != "" {
		return Result{}, runtimeError(ready.Error)
	}
	if ready.Phase != "ready" || ready.Result != nil {
		return Result{}, ErrProtocol
	}
	if before != nil {
		if err := before(ctx); err != nil {
			return Result{}, err
		}
	}
	if err := writeRuntimeFrame(conn, runtimeReply{Phase: "dispatch"}, 1024); err != nil {
		return Result{}, err
	}
	var completed runtimeReply
	if err := readRuntimeFrame(reader, &completed); err != nil {
		return Result{}, err
	}
	if completed.Error != "" {
		return Result{}, runtimeError(completed.Error)
	}
	if ctx.Err() != nil {
		return Result{}, ErrBudget
	}
	if completed.Phase != "complete" || completed.Result == nil || completed.Result.ModelID != model || completed.Result.StopReason != "end_turn" || completed.Result.UsageState != "unknown" || len(completed.Result.Text) == 0 || len(completed.Result.Text) > 64<<10 {
		return Result{}, ErrProtocol
	}
	return *completed.Result, nil
}

func ServeRuntime(ctx context.Context, listener net.Listener, process AntigravityProcess, model string, cleanup func() error) error {
	if process.Validate() != nil || !strings.HasPrefix(model, "gemini-") || cleanup == nil {
		return ErrProcess
	}
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	slot := make(chan struct{}, 1)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return ErrProcess
		}
		select {
		case slot <- struct{}{}:
			go func() {
				defer func() { <-slot }()
				serveRuntimeTurn(ctx, conn, model, func(turn context.Context, prompt string, before func(context.Context) error) (Result, error) {
					return RunAntigravity(turn, process, model, prompt, before)
				}, cleanup)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func serveRuntimeTurn(parent context.Context, conn net.Conn, model string, execute func(context.Context, string, func(context.Context) error) (Result, error), cleanup func() error) {
	defer conn.Close()
	ctx, cancel := context.WithTimeout(parent, 300*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if conn.SetDeadline(deadline) != nil {
		return
	}
	reader := bufio.NewReaderSize(conn, 4<<20)
	var request runtimeRequest
	if readRuntimeFrame(reader, &request) != nil || request.Model != model || len(request.Prompt) == 0 || len(request.Prompt) > 2<<20 {
		return
	}
	watchDone := make(chan struct{})
	watching := false
	dispatched := false
	result, err := execute(ctx, request.Prompt, func(context.Context) error {
		if err := writeRuntimeFrame(conn, runtimeReply{Phase: "ready"}, 1024); err != nil {
			return err
		}
		var dispatch runtimeReply
		if readRuntimeFrame(reader, &dispatch) != nil || dispatch.Phase != "dispatch" || dispatch.Result != nil || dispatch.Error != "" {
			return ErrProtocol
		}
		watching = true
		dispatched = true
		go func() {
			defer close(watchDone)
			_, _ = reader.ReadByte()
			cancel()
		}()
		return nil
	})
	cleanupErr := cleanup()
	if cleanupErr != nil {
		err = ErrProcess
	}
	if err == nil && ctx.Err() == nil {
		_ = writeRuntimeFrame(conn, runtimeReply{Phase: "complete", Result: &result}, 1<<20)
	} else {
		code := "process"
		switch {
		case errors.Is(err, ErrAuthenticationRequired):
			code = "auth_required"
		case errors.Is(err, ErrTools):
			code = "tools"
		case errors.Is(err, ErrBudget), ctx.Err() != nil:
			code = "budget"
		case errors.Is(err, ErrModel):
			code = "model"
		case errors.Is(err, ErrProtocol):
			code = "protocol"
		case errors.Is(err, ErrRemote):
			code = "remote"
		}
		log.Printf("ACP turn refused: dispatched=%t class=%s cleanup_failed=%t", dispatched, code, cleanupErr != nil)
		var protocolFailure *ProtocolFailure
		if errors.As(err, &protocolFailure) {
			log.Print(protocolFailure.Error())
		}
		_ = writeRuntimeFrame(conn, runtimeReply{Phase: "failed", Error: code}, 1024)
	}
	_ = conn.Close()
	if watching {
		<-watchDone
	}
}

func readRuntimeFrame(reader *bufio.Reader, value any) error {
	raw, err := reader.ReadSlice('\n')
	if err != nil || jsonstrict.Decode(raw, value) != nil {
		return ErrProtocol
	}
	return nil
}

func writeRuntimeFrame(conn net.Conn, value any, limit int) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) >= limit {
		return ErrBudget
	}
	raw = append(raw, '\n')
	for len(raw) > 0 {
		written, err := conn.Write(raw)
		if err != nil || written == 0 {
			return ErrProtocol
		}
		raw = raw[written:]
	}
	return nil
}

func runtimeError(code string) error {
	switch code {
	case "auth_required":
		return ErrAuthenticationRequired
	case "tools":
		return ErrTools
	case "budget":
		return ErrBudget
	case "model":
		return ErrModel
	case "process":
		return ErrProcess
	case "protocol":
		return ErrProtocol
	case "remote":
		return ErrRemote
	default:
		return ErrProtocol
	}
}
