package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"time"
	"unicode/utf8"
)

func (probe *Probe) Run(parent context.Context, conn net.Conn, request Request) (result Result, err error) {
	if conn == nil {
		return Result{}, ErrProtocol
	}
	defer conn.Close()
	if err := probe.claim(request); err != nil {
		return Result{}, err
	}
	return probe.run(parent, conn, request, "/acp-fixture")
}

func (probe *Probe) claim(request Request) error {
	if request.Binding != probe.binding {
		return ErrBinding
	}
	if request.ResumeSessionID != "" {
		return ErrResume
	}
	promptLimit := probe.limits.PromptBytes
	if promptLimit == 0 {
		promptLimit = probe.limits.TextBytes
	}
	if request.ModelID == "" || len(request.ModelID) > 256 || !utf8.ValidString(request.ModelID) || request.Prompt == "" || len(request.Prompt) > promptLimit || !utf8.ValidString(request.Prompt) {
		return ErrProtocol
	}
	if !probe.used.CompareAndSwap(false, true) {
		return ErrConsumed
	}
	return nil
}

func (probe *Probe) run(parent context.Context, conn net.Conn, request Request, cwd string) (result Result, err error) {
	ctx, cancel := context.WithTimeout(parent, probe.limits.WallTime)
	defer cancel()
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	deadline, _ := ctx.Deadline()
	if conn.SetDeadline(deadline) != nil {
		return Result{}, ErrProtocol
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
		close(interrupted)
	})
	wire := &exchange{conn: conn, reader: bufio.NewReaderSize(conn, 256*1024), limits: probe.limits}
	defer func() {
		if !stop() {
			<-interrupted
		}
		if parent.Err() != nil {
			err = parent.Err()
		} else if parentDeadline, exists := parent.Deadline(); exists && !time.Now().Before(parentDeadline) {
			err = context.DeadlineExceeded
		} else if ctx.Err() != nil || !time.Now().Before(deadline) {
			err = ErrBudget
		}
		if err != nil {
			result = Result{}
			if wire.sessionID != "" {
				_ = conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
				_ = json.NewEncoder(conn).Encode(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]string{"sessionId": wire.sessionID}})
			}
		}
	}()
	var initialized struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err = wire.call("initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]string{"name": "butler", "version": "0.1"},
		"clientCapabilities": map[string]any{"fs": map[string]bool{"readTextFile": false, "writeTextFile": false}, "terminal": false},
	}, &initialized); err != nil {
		return Result{}, err
	}
	if initialized.ProtocolVersion != 1 {
		return Result{}, ErrProtocol
	}
	var session struct {
		SessionID     string         `json:"sessionId"`
		ConfigOptions []configOption `json:"configOptions"`
	}
	if err = wire.call("session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}, &session); err != nil {
		return Result{}, err
	}
	if session.SessionID == "" || len(session.SessionID) > 256 {
		return Result{}, ErrProtocol
	}
	wire.sessionID = session.SessionID
	optionID := ""
	modelOptions := 0
	for _, option := range session.ConfigOptions {
		if option.Category != "model" || option.Type != "select" {
			continue
		}
		modelOptions++
		for _, choice := range option.Options {
			if choice.Value == request.ModelID {
				optionID = option.ID
			}
		}
	}
	if modelOptions != 1 || optionID == "" {
		return Result{}, ErrModel
	}
	var selected struct {
		ConfigOptions []configOption `json:"configOptions"`
	}
	if err = wire.call("session/set_config_option", map[string]string{"sessionId": wire.sessionID, "configId": optionID, "value": request.ModelID}, &selected); err != nil {
		return Result{}, err
	}
	confirmed := 0
	for _, option := range selected.ConfigOptions {
		if option.ID == optionID {
			if option.Category != "model" || option.Type != "select" || option.CurrentValue != request.ModelID {
				return Result{}, ErrModel
			}
			confirmed++
		}
	}
	if confirmed != 1 {
		return Result{}, ErrModel
	}
	wire.modelID, wire.modelOptionID = request.ModelID, optionID
	if request.BeforePrompt != nil {
		if err := request.BeforePrompt(ctx); err != nil {
			return Result{}, err
		}
	}
	wire.prompting = true
	var prompt struct {
		StopReason string `json:"stopReason"`
	}
	if err = wire.call("session/prompt", map[string]any{"sessionId": wire.sessionID, "prompt": []any{map[string]string{"type": "text", "text": request.Prompt}}}, &prompt); err != nil {
		return Result{}, err
	}
	if prompt.StopReason != "end_turn" || wire.text.Len() == 0 {
		return Result{}, ErrProtocol
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	return Result{Text: wire.text.String(), ModelID: request.ModelID, StopReason: prompt.StopReason, UsageState: "unknown"}, nil
}
