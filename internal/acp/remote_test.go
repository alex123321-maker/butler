package acp

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeDispatchAndCleanupPrecedeOutput(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		client, server := net.Pipe()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var prompted, cleaned atomic.Bool
		done := make(chan struct{})
		go func() {
			defer close(done)
			serveRuntimeTurn(ctx, server, "gemini-fixture", func(turn context.Context, prompt string, before func(context.Context) error) (Result, error) {
				if prompt != "fixture" {
					return Result{}, ErrProtocol
				}
				if err := before(turn); err != nil {
					return Result{}, err
				}
				prompted.Store(true)
				return Result{Text: "fixture", ModelID: "gemini-fixture", StopReason: "end_turn", UsageState: "unknown"}, nil
			}, func() error {
				cleaned.Store(true)
				if cleanupFails {
					return ErrProcess
				}
				return nil
			})
		}()
		if err := writeRuntimeFrame(client, runtimeRequest{Model: "gemini-fixture", Prompt: "fixture"}, 4096); err != nil {
			t.Fatal(err)
		}
		reader := bufio.NewReader(client)
		var reply runtimeReply
		if readRuntimeFrame(reader, &reply) != nil || reply.Phase != "ready" || prompted.Load() {
			t.Fatal("prompt crossed the dispatch gate")
		}
		if writeRuntimeFrame(client, runtimeReply{Phase: "dispatch"}, 1024) != nil || readRuntimeFrame(reader, &reply) != nil {
			t.Fatal("runtime exchange failed")
		}
		if !prompted.Load() || !cleaned.Load() || cleanupFails && (reply.Error != "process" || reply.Result != nil) || !cleanupFails && (reply.Phase != "complete" || reply.Result == nil) {
			t.Fatal("result escaped namespace cleanup", reply)
		}
		client.Close()
		<-done
		cancel()
	}
}

func TestRuntimeRevocationCancelsBeforePrompt(t *testing.T) {
	client, server := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var prompted, cleaned atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveRuntimeTurn(ctx, server, "gemini-fixture", func(turn context.Context, _ string, before func(context.Context) error) (Result, error) {
			if err := before(turn); err != nil {
				return Result{}, err
			}
			prompted.Store(true)
			return Result{}, nil
		}, func() error { cleaned.Store(true); return nil })
	}()
	if writeRuntimeFrame(client, runtimeRequest{Model: "gemini-fixture", Prompt: "fixture"}, 4096) != nil {
		t.Fatal("request failed")
	}
	var reply runtimeReply
	if readRuntimeFrame(bufio.NewReader(client), &reply) != nil || reply.Phase != "ready" {
		t.Fatal("dispatch gate unavailable")
	}
	client.Close()
	<-done
	if prompted.Load() || !cleaned.Load() {
		t.Fatal("revocation launched inference or skipped cleanup")
	}
}

func TestRuntimeDisconnectCancelsActiveInference(t *testing.T) {
	client, server := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveRuntimeTurn(ctx, server, "gemini-fixture", func(turn context.Context, _ string, before func(context.Context) error) (Result, error) {
			if err := before(turn); err != nil {
				return Result{}, err
			}
			close(started)
			<-turn.Done()
			return Result{}, turn.Err()
		}, func() error { return nil })
	}()
	writeRuntimeFrame(client, runtimeRequest{Model: "gemini-fixture", Prompt: "fixture"}, 4096)
	var reply runtimeReply
	readRuntimeFrame(bufio.NewReader(client), &reply)
	writeRuntimeFrame(client, runtimeReply{Phase: "dispatch"}, 1024)
	<-started
	client.Close()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("disconnect did not cancel active inference")
	}
}

func TestRuntimeFramesRejectExtensionsAndOversize(t *testing.T) {
	for _, raw := range []string{"{\"phase\":\"dispatch\",\"token\":\"not-a-secret\"}\n", "{\"phase\":\"dispatch\",\"phase\":\"complete\"}\n"} {
		var reply runtimeReply
		if readRuntimeFrame(bufio.NewReaderSize(strings.NewReader(raw), 4096), &reply) == nil {
			t.Fatal("untrusted runtime frame accepted")
		}
	}
	if !errors.Is(runtimeError("raw private error"), ErrProtocol) {
		t.Fatal("raw runtime errors passed through")
	}
}
