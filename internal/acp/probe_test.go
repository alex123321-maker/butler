package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func fixtureBinding() Binding {
	return Binding{OwnerID: "owner", SessionID: "session", TaskID: "task", RunID: "run", TaskVersion: 1, AuthorityVersion: 2, ConstraintVersion: 3}
}

func fixtureLimits() Limits {
	return Limits{WallTime: time.Second, Messages: 64, WireBytes: 1024 * 1024, TextBytes: 4096}
}

func fixtureRequest() Request {
	return Request{Binding: fixtureBinding(), ModelID: "fixture-gemini", Prompt: "Synthetic fixture only"}
}

func newFixtureProbe(test *testing.T, limits Limits) *Probe {
	test.Helper()
	probe, err := NewProbe(fixtureBinding(), limits)
	if err != nil {
		test.Fatal(err)
	}
	return probe
}

type peer struct {
	conn    net.Conn
	decoder *json.Decoder
	encoder *json.Encoder
	cwd     string
}

func (agent *peer) read(method string) (message, error) {
	var request message
	if err := agent.decoder.Decode(&request); err != nil {
		return request, err
	}
	if request.Method != method {
		return request, fmt.Errorf("wanted %s, got %s", method, request.Method)
	}
	return request, nil
}

func (agent *peer) reply(request message, result any) error {
	return agent.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func (agent *peer) update(sessionID, kind, text string) error {
	return agent.encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": sessionID, "update": map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": text}}}})
}

func (agent *peer) handshake(model, selected string) error {
	request, err := agent.read("initialize")
	if err != nil {
		return err
	}
	var params struct {
		ClientCapabilities struct {
			FS       map[string]bool `json:"fs"`
			Terminal bool            `json:"terminal"`
		} `json:"clientCapabilities"`
	}
	if json.Unmarshal(request.Params, &params) != nil || params.ClientCapabilities.Terminal || params.ClientCapabilities.FS["readTextFile"] || params.ClientCapabilities.FS["writeTextFile"] {
		return errors.New("unsafe client capabilities")
	}
	if err := agent.reply(request, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]bool{"loadSession": true}}); err != nil {
		return err
	}
	request, err = agent.read("session/new")
	if err != nil {
		return err
	}
	var session struct {
		CWD        string `json:"cwd"`
		MCPServers []any  `json:"mcpServers"`
	}
	expectedCWD := agent.cwd
	if expectedCWD == "" {
		expectedCWD = "/acp-fixture"
	}
	if json.Unmarshal(request.Params, &session) != nil || session.CWD != expectedCWD || session.MCPServers == nil || len(session.MCPServers) != 0 {
		return errors.New("unexpected session configuration")
	}
	options := func(value string) []any {
		return []any{map[string]any{"id": "model", "category": "model", "type": "select", "currentValue": value, "options": []any{map[string]string{"value": model, "name": "Fixture"}}}}
	}
	if err := agent.reply(request, map[string]any{"sessionId": "agent-session", "configOptions": options(model)}); err != nil {
		return err
	}
	if model != fixtureRequest().ModelID {
		_, err = agent.read("session/cancel")
		return err
	}
	request, err = agent.read("session/set_config_option")
	if err != nil {
		return err
	}
	var config map[string]string
	if json.Unmarshal(request.Params, &config) != nil || config["value"] != fixtureRequest().ModelID || config["configId"] != "model" || config["sessionId"] != "agent-session" {
		return errors.New("wrong model selection")
	}
	return agent.reply(request, map[string]any{"configOptions": options(selected)})
}

func runPeer(test *testing.T, probe *Probe, ctx context.Context, request Request, serve func(*peer) error) (Result, error) {
	test.Helper()
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		_ = server.SetDeadline(time.Now().Add(3 * time.Second))
		agent := &peer{conn: server, decoder: json.NewDecoder(server), encoder: json.NewEncoder(server)}
		err := serve(agent)
		if err == nil {
			_, _ = io.Copy(io.Discard, server)
		}
		done <- err
	}()
	result, err := probe.Run(ctx, client, request)
	if peerError := <-done; peerError != nil {
		test.Fatal(peerError)
	}
	return result, err
}

func promptPeer(serve func(*peer, message) error) func(*peer) error {
	return func(agent *peer) error {
		if err := agent.handshake("fixture-gemini", "fixture-gemini"); err != nil {
			return err
		}
		request, err := agent.read("session/prompt")
		if err != nil {
			return err
		}
		return serve(agent, request)
	}
}

func TestProbeExactModelAndUnknownUsage(test *testing.T) {
	probe := newFixtureProbe(test, fixtureLimits())
	result, err := runPeer(test, probe, context.Background(), fixtureRequest(), promptPeer(func(agent *peer, request message) error {
		for _, kind := range []string{"agent_thought_chunk", "usage_update", "plan"} {
			if err := agent.update("agent-session", kind, "private-not-output"); err != nil {
				return err
			}
		}
		for _, text := range []string{"Hello ", "Gemini"} {
			if err := agent.update("agent-session", "agent_message_chunk", text); err != nil {
				return err
			}
		}
		return agent.reply(request, map[string]any{"stopReason": "end_turn", "usage": map[string]int{"totalTokens": 25}})
	}))
	if err != nil || result.Text != "Hello Gemini" || result.ModelID != "fixture-gemini" || result.UsageState != "unknown" || result.StopReason != "end_turn" {
		test.Fatalf("result=%+v err=%v", result, err)
	}
	_, err = runPeer(test, probe, context.Background(), fixtureRequest(), func(agent *peer) error {
		var input message
		if err := agent.decoder.Decode(&input); err != io.EOF {
			return errors.New("consumed instance wrote to peer")
		}
		return nil
	})
	if !errors.Is(err, ErrConsumed) {
		test.Fatal(err)
	}
}

func TestProbeRejectsStaleBindingsAndResumeBeforeWrite(test *testing.T) {
	mutations := map[string]func(*Request){
		"owner":        func(request *Request) { request.Binding.OwnerID = "other" },
		"session":      func(request *Request) { request.Binding.SessionID = "other" },
		"task":         func(request *Request) { request.Binding.TaskID = "other" },
		"run":          func(request *Request) { request.Binding.RunID = "other" },
		"task-version": func(request *Request) { request.Binding.TaskVersion++ },
		"authority":    func(request *Request) { request.Binding.AuthorityVersion++ },
		"constraints":  func(request *Request) { request.Binding.ConstraintVersion++ },
		"resume":       func(request *Request) { request.ResumeSessionID = "old-provider-session" },
	}
	for name, mutate := range mutations {
		test.Run(name, func(test *testing.T) {
			request := fixtureRequest()
			mutate(&request)
			_, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), request, func(agent *peer) error {
				var input message
				if err := agent.decoder.Decode(&input); err != io.EOF {
					return errors.New("stale binding wrote to peer")
				}
				return nil
			})
			want := ErrBinding
			if name == "resume" {
				want = ErrResume
			}
			if !errors.Is(err, want) {
				test.Fatalf("got %v want %v", err, want)
			}
		})
	}
}

func TestProbeDeniesEveryClientMethod(test *testing.T) {
	for _, method := range []string{"session/request_permission", "fs/read_text_file", "fs/write_text_file", "terminal/create", "terminal/output", "secrets/get", "unknown/extension"} {
		test.Run(method, func(test *testing.T) {
			result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), fixtureRequest(), promptPeer(func(agent *peer, request message) error {
				if err := agent.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "agent-request", "method": method, "params": map[string]any{"sessionId": "agent-session", "options": []any{map[string]string{"optionId": "allow", "kind": "allow_always"}}}}); err != nil {
					return err
				}
				var response message
				if err := agent.decoder.Decode(&response); err != nil {
					return err
				}
				if string(response.ID) != `"agent-request"` {
					return errors.New("wrong request correlation")
				}
				if method == "session/request_permission" {
					if string(response.Result) != `{"outcome":{"outcome":"cancelled"}}` {
						return errors.New("permission was not cancelled")
					}
				} else if !strings.Contains(string(response.Error), "-32601") {
					return errors.New("client method not denied")
				}
				_, err := agent.read("session/cancel")
				return err
			}))
			if !errors.Is(err, ErrTools) || result != (Result{}) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}
