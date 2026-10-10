// provider-auth is a private operator utility. Credentials arrive only on stdin;
// they are never flags, chat messages or model-visible arguments.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	action := flag.String("action", "state", "Antigravity operator action: import, resolve, state, disconnect, smoke")
	flag.Parse()
	if err := execute(*action); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func execute(action string) error {
	if strings.TrimSpace(os.Getenv(config.SettingsEncryptionKeyEnv)) == "" {
		return fmt.Errorf("provider auth requires BUTLER_SETTINGS_ENCRYPTION_KEY")
	}
	dsn := os.Getenv("BUTLER_POSTGRES_URL")
	if dsn == "" {
		return fmt.Errorf("provider auth requires BUTLER_POSTGRES_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("provider auth database configuration failed")
	}
	defer pool.Close()
	if pool.Ping(ctx) != nil {
		return fmt.Errorf("provider auth database connection failed")
	}
	m := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	switch action {
	case "import":
		if err := m.ImportAntigravity(ctx, os.Stdin, "private-operator-import"); err != nil {
			return err
		}
		fmt.Println("Antigravity credentials imported into encrypted PostgreSQL storage.")
	case "resolve":
		if _, err := m.ResolveAntigravity(ctx); err != nil {
			return err
		}
		fmt.Println("Antigravity access token and subscription project resolved.")
	case "state":
		state, err := m.State(ctx, modelprovider.ProviderAntigravity)
		if err != nil {
			return err
		}
		fmt.Printf("Antigravity credentials stored: %t. Model quota is checked by a requested turn.\n", state.Connected)
	case "disconnect":
		if m.Delete(ctx, modelprovider.ProviderAntigravity) != nil {
			return fmt.Errorf("provider auth disconnect failed")
		}
		fmt.Println("Antigravity credentials removed.")
	case "smoke":
		return smoke(ctx, m)
	default:
		return fmt.Errorf("unknown provider auth operator action")
	}
	return nil
}
func smoke(ctx context.Context, source providerauth.AntigravityTokenSource) error {
	p, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.DefaultModel, AuthSource: source, Timeout: 2 * time.Minute})
	if err != nil {
		return err
	}
	base := transport.TransportRunContext{RunID: "antigravity-native-smoke-text", SessionKey: "operator:antigravity-smoke", ProviderName: antigravity.Name, ModelName: antigravity.DefaultModel}
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: base, InputItems: []transport.InputItem{{Role: "user", Content: "Ответь коротко по-русски: что такое контрольная сумма?"}}, StreamingEnabled: true})
	if err != nil {
		return err
	}
	answer, proposal, err := collect(stream)
	if err != nil {
		return err
	}
	if answer == "" || proposal != nil {
		return fmt.Errorf("native text smoke returned no answer")
	}
	fmt.Println("Native Gemini 3.8 Flash text turn: passed.")
	base.RunID = "antigravity-native-smoke-proposal"
	stream, err = p.StartRun(ctx, transport.StartRunRequest{Context: base, InputItems: []transport.InputItem{{Role: "user", Content: "Вызови доступную функцию web.fetch для https://example.com. Butler выполнит её после проверки; сам ничего не выполняй."}}, ToolDefinitions: []transport.ToolDefinition{{Name: "web.fetch", Description: "Fetch public webpage through Butler Tool Broker", SchemaJSON: `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"],"additionalProperties":false}`}}, StreamingEnabled: true})
	if err != nil {
		return err
	}
	_, proposal, err = collect(stream)
	if err != nil {
		return err
	}
	if proposal == nil || proposal.ToolName != "web.fetch" {
		return fmt.Errorf("native function smoke returned no expected proposal")
	}
	fmt.Println("Native function proposal: passed; no tool executed.")
	stream, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: base.RunID, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"success","result":{"title":"Example Domain","source":"synthetic operator test observation; no web request was made"}}`})
	if err != nil {
		return err
	}
	answer, next, err := collect(stream)
	if err != nil {
		return err
	}
	if answer == "" || next != nil {
		return fmt.Errorf("native function-result continuation did not finish")
	}
	fmt.Println("Native model continuation with synthetic result: passed.")
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
