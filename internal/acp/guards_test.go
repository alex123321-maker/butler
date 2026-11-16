package acp

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProbeCancellationIsBounded(test *testing.T) {
	for _, ignoreCancel := range []bool{false, true} {
		test.Run(map[bool]string{false: "receives-cancel", true: "unresponsive-peer"}[ignoreCancel], func(test *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := time.Now()
			result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), ctx, fixtureRequest(), promptPeer(func(agent *peer, request message) error {
				cancel()
				if ignoreCancel {
					time.Sleep(150 * time.Millisecond)
					return nil
				}
				cancelRequest, err := agent.read("session/cancel")
				if err != nil {
					return err
				}
				if !strings.Contains(string(cancelRequest.Params), "agent-session") {
					return errors.New("cancel used wrong session")
				}
				return nil
			}))
			if !errors.Is(err, context.Canceled) || result != (Result{}) || time.Since(started) > time.Second {
				test.Fatalf("result=%+v err=%v duration=%v", result, err, time.Since(started))
			}
		})
	}
}

func TestProbeWallTimeAndParentDeadline(test *testing.T) {
	for _, parentDeadline := range []bool{false, true} {
		test.Run(map[bool]string{false: "child-limit", true: "parent-earlier"}[parentDeadline], func(test *testing.T) {
			limits := fixtureLimits()
			limits.WallTime = 100 * time.Millisecond
			ctx := context.Background()
			want := ErrBudget
			if parentDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 25*time.Millisecond)
				defer cancel()
				want = context.DeadlineExceeded
			}
			started := time.Now()
			_, err := runPeer(test, newFixtureProbe(test, limits), ctx, fixtureRequest(), func(agent *peer) error {
				_, err := agent.read("initialize")
				if err != nil {
					return err
				}
				var buffer [1]byte
				_, err = agent.conn.Read(buffer[:])
				if err != io.EOF {
					return errors.New("timeout did not close connection")
				}
				return nil
			})
			if !errors.Is(err, want) || time.Since(started) > time.Second {
				test.Fatalf("got %v want %v", err, want)
			}
		})
	}
}

func TestProbeRejectsInvalidProtocol(test *testing.T) {
	cases := map[string]string{
		"aliased-envelope":   `{"JSONRPC":"2.0","id":1,"result":{"protocolVersion":1}}`,
		"method-in-response": `{"jsonrpc":"2.0","id":1,"method":"","result":{"protocolVersion":1}}`,
		"params-in-response": `{"jsonrpc":"2.0","id":1,"params":{},"result":{"protocolVersion":1}}`,
		"null-request-id":    `{"jsonrpc":"2.0","id":null,"method":"fs/read_text_file","params":{}}`,
		"version":            `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":2}}`,
		"wrong-id":           `{"jsonrpc":"2.0","id":20,"result":{"protocolVersion":1}}`,
		"both":               `{"jsonrpc":"2.0","id":1,"result":{},"error":{}}`,
		"duplicate":          `{"jsonrpc":"2.0","id":1,"id":1,"result":{"protocolVersion":1}}`,
		"null":               `{"jsonrpc":"2.0","id":1,"result":null}`,
		"non-json":           `agent log text`,
		"invalid-utf8":       "\xff",
		"auth-error":         `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"secret-canary","data":"token-canary"}}`,
		"oversize":           strings.Repeat("x", 256*1024),
	}
	for name, raw := range cases {
		test.Run(name, func(test *testing.T) {
			result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), fixtureRequest(), func(agent *peer) error {
				if _, err := agent.read("initialize"); err != nil {
					return err
				}
				_, _ = io.WriteString(agent.conn, raw+"\n")
				return nil
			})
			want := ErrProtocol
			if name == "auth-error" {
				want = ErrRemote
			}
			if name == "oversize" {
				want = ErrBudget
			}
			if !errors.Is(err, want) || result != (Result{}) || strings.Contains(err.Error(), "canary") {
				test.Fatalf("result=%+v err=%v want=%v", result, err, want)
			}
		})
	}
}

func TestProbeRequiresExactModelBeforePrompt(test *testing.T) {
	for _, name := range []string{"not-offered", "not-confirmed"} {
		test.Run(name, func(test *testing.T) {
			_, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), fixtureRequest(), func(agent *peer) error {
				model := "fixture-gemini"
				if name == "not-offered" {
					model = "other-model"
				}
				if err := agent.handshake(model, "other-model"); err != nil {
					return err
				}
				if name == "not-confirmed" {
					_, err := agent.read("session/cancel")
					return err
				}
				return nil
			})
			if !errors.Is(err, ErrModel) {
				test.Fatal(err)
			}
		})
	}
}

func TestProbeRejectsToolActivityAndForeignUpdates(test *testing.T) {
	for _, kind := range []string{"tool_call", "tool_call_update", "agent_message_chunk", "config_option_update", "future-unknown-kind"} {
		test.Run(kind, func(test *testing.T) {
			result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), fixtureRequest(), promptPeer(func(agent *peer, request message) error {
				if err := agent.update("agent-session", "agent_message_chunk", "partial-must-be-discarded"); err != nil {
					return err
				}
				session := "agent-session"
				if kind == "agent_message_chunk" {
					session = "another-session"
				}
				if err := agent.update(session, kind, "untrusted"); err != nil {
					return err
				}
				_, err := agent.read("session/cancel")
				return err
			}))
			want := ErrProtocol
			if kind == "tool_call" || kind == "tool_call_update" {
				want = ErrTools
			}
			if !errors.Is(err, want) || result != (Result{}) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestProbeDoesNotPromoteIncompleteTurns(test *testing.T) {
	for _, reason := range []string{"cancelled", "max_tokens", "max_turn_requests", "refusal", "future-value", ""} {
		test.Run(reason, func(test *testing.T) {
			result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), fixtureRequest(), promptPeer(func(agent *peer, request message) error {
				if err := agent.update("agent-session", "agent_message_chunk", "partial"); err != nil {
					return err
				}
				return agent.reply(request, map[string]string{"stopReason": reason})
			}))
			if !errors.Is(err, ErrProtocol) || result != (Result{}) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestProbeProtocolBudgets(test *testing.T) {
	for _, name := range []string{"messages", "bytes", "text"} {
		test.Run(name, func(test *testing.T) {
			limits := fixtureLimits()
			if name == "messages" {
				limits.Messages = 12
			}
			if name == "bytes" {
				limits.WireBytes = 4096
			}
			if name == "text" {
				limits.TextBytes = 32
			}
			result, err := runPeer(test, newFixtureProbe(test, limits), context.Background(), fixtureRequest(), promptPeer(func(agent *peer, request message) error {
				for index := 0; index < 16; index++ {
					if err := agent.update("agent-session", "agent_message_chunk", strings.Repeat("x", 1000)); err != nil {
						break
					}
				}
				return nil
			}))
			if !errors.Is(err, ErrBudget) || result != (Result{}) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestProbeRejectsUnboundedLimits(test *testing.T) {
	for _, limits := range []Limits{{}, {WallTime: 301 * time.Second, Messages: 1, WireBytes: 1, TextBytes: 1}, {WallTime: time.Second, Messages: 513, WireBytes: 1, TextBytes: 1}, {WallTime: time.Second, Messages: 1, WireBytes: 4*1024*1024 + 1, TextBytes: 1}, {WallTime: time.Second, Messages: 1, WireBytes: 1, TextBytes: 65537}} {
		if _, err := NewProbe(fixtureBinding(), limits); !errors.Is(err, ErrBudget) {
			test.Fatal(err)
		}
	}
	if _, err := NewProbe(Binding{}, fixtureLimits()); !errors.Is(err, ErrBinding) {
		test.Fatal(err)
	}
}
