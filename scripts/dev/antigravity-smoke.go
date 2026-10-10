// Build on the lab, then run in the isolated Antigravity profile container.
// No authorization code or token is accepted by this program.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func main() {
	if err := smoke(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func smoke() error {
	p, err := antigravity.NewProvider(antigravity.Config{BinaryPath: "/opt/antigravity/agy", ProfileDir: "/home/butler", Timeout: 2 * time.Minute})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	base := transport.TransportRunContext{RunID: "antigravity-smoke-text", SessionKey: "smoke:antigravity", ProviderName: antigravity.Name, ModelName: "gemini-3.8-flash-tiered"}
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: base, InputItems: []transport.InputItem{{Role: "user", Content: "Ответь коротко по-русски: что такое контрольная сумма? Используй answer, пустой tool_name и пустой arguments."}}})
	if err != nil {
		return err
	}
	answer, _, err := collect(stream)
	if err != nil {
		return err
	}
	if answer == "" {
		return fmt.Errorf("real Antigravity text turn returned no answer")
	}
	fmt.Println("Real text turn: passed; empty CLI toolkit confirmed.")
	base.RunID = "antigravity-smoke-proposal"
	stream, err = p.StartRun(ctx, transport.StartRunRequest{Context: base, InputItems: []transport.InputItem{{Role: "user", Content: "Предложи вызов web.fetch для https://example.com, чтобы Butler сам получил страницу. Верни kind tool_call, пустой answer и arguments с url. Здесь ничего не выполняй."}}, ToolDefinitions: []transport.ToolDefinition{{Name: "web.fetch", Description: "Fetch a public web page through Butler Tool Broker", SchemaJSON: `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"],"additionalProperties":false}`}}})
	if err != nil {
		return err
	}
	_, proposal, err := collect(stream)
	if err != nil {
		return err
	}
	if proposal == nil || proposal.ToolName != "web.fetch" {
		return fmt.Errorf("real Antigravity turn returned no expected tool proposal")
	}
	fmt.Println("Real tool proposal: passed; no CLI tool executed.")
	stream, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: base.RunID, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"success","result":{"title":"Example Domain","source":"synthetic adapter test observation; no actual web request was made"}}`})
	if err != nil {
		return err
	}
	answer, next, err := collect(stream)
	if err != nil {
		return err
	}
	if answer == "" || next != nil {
		return fmt.Errorf("real Antigravity tool-result continuation did not finish")
	}
	fmt.Println("Real model continuation with synthetic tool result: passed.")
	return nil
}

func collect(stream transport.EventStream) (string, *transport.ToolCallRequest, error) {
	var answer string
	var proposal *transport.ToolCallRequest
	for event := range stream {
		if event.TransportError != nil {
			return "", nil, event.TransportError
		}
		if event.AssistantFinal != nil {
			answer = event.AssistantFinal.Content
		}
		if event.ToolCall != nil {
			proposal = event.ToolCall
		}
	}
	return answer, proposal, nil
}
