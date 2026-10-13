package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
)

func TestReleaseChainV6WindowNoRefillOrObserverDowngradeIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	pin, history := strings.Repeat("a", 64), strings.Repeat("f", 64)
	var schema string
	if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		t.Fatal("synthetic schema unavailable")
	}
	deadline := time.Now().UTC().Add(time.Minute)
	w, err := createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 537, schema, deadline, 78, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		max    int
		stages bool
	}{{"legacy_default80", 80, true}, {"disabled_observer", 78, false}, {"refill79", 79, true}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 537, schema, deadline.Add(time.Minute), tc.max, tc.stages); err == nil {
				t.Fatal("reconstructed scope enlarged or observer disabled")
			}
		})
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	legacy := *w
	legacy.maximum = 0
	if _, _, err = legacy.reserve(ctx, "normal_sent", "start", "synthetic-root", "synthetic-run", nil, guard); err == nil {
		t.Fatal("compatibility fallback80 charged a V6 entry")
	}
	var count atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 90; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if _, _, err := w.reserve(ctx, releaseChainV5Scenarios()[i%5], "start", "synthetic-root", "synthetic-run", i, guard); err == nil {
				count.Add(1)
			}
		}(i)
	}
	workers.Wait()
	if count.Load() != 78 {
		t.Fatal("V5 spent entries refunded or cap lost", count.Load())
	}
	var attempts, maximum, rows int
	var stages bool
	var stored time.Time
	if pool.QueryRow(ctx, `SELECT attempts,maximum_new_attempts,provider_stage_observation,expires_at FROM fixture_chain_v5_high_window`).Scan(&attempts, &maximum, &stages, &stored) != nil || attempts != 615 || maximum != 78 || !stages || !stored.Equal(deadline.Truncate(time.Microsecond)) || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != 78 {
		t.Fatal("durable scope bounds or rows differ")
	}
	restarted, err := createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 537, schema, deadline.Add(time.Minute), 78, true)
	if err != nil {
		t.Fatal("matching restart could not reconstruct the spent scope")
	}
	if _, _, err = restarted.reserve(ctx, "restart_sent", "start", "another-root", "another-run", nil, guard); err == nil {
		t.Fatal("restart refilled allowance")
	}
	for _, query := range []string{`UPDATE fixture_chain_v5_high_window SET attempts=538`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,maximum_new_attempts=80`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,provider_stage_observation=false`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,expires_at=expires_at+interval '1 hour'`} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("sealed bounds changed")
		}
	}
}

type releaseChainV6StageSpy struct {
	*releaseChainV5StreamSpy
	stage transport.ProviderStage
	err   error
}

func (p *releaseChainV6StageSpy) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	transport.ObserveProviderStage(ctx, p.stage)
	if p.err != nil {
		p.calls.Add(1)
		return nil, p.err
	}
	return p.releaseChainV5StreamSpy.StartRun(ctx, r)
}

func releaseChainV6OwnedProviderFixture(t *testing.T, provider transport.ModelProvider) (*releaseChainV5Provider, context.Context, transport.StartRunRequest) {
	t.Helper()
	p, live, req, _ := releaseChainV5OwnedProviderFixture(t, provider)
	schema := p.window.schema + "_v6"
	w, err := createReleaseChainHighWindowInSchemaAt(context.Background(), p.window.pool, strings.Repeat("d", 64), strings.Repeat("e", 64), 537, schema, time.Now().UTC().Add(time.Minute), 78, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = p.window.pool.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	})
	p.window = w
	return p, live, req
}

func TestReleaseChainV6ProviderStageMeasuredBeforeActionIntegration(t *testing.T) {
	version := "gemini-3.8-flash-n"
	usage := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	event := transport.NewToolCallRequestedEvent("chain-run", antigravity.Name, transport.ToolCallRequest{ToolName: "synthetic.read", ArgsJSON: `{}`})
	event.Usage, event.ProviderModelVersion = &usage, &version
	for _, tc := range []struct {
		name    string
		stage   transport.ProviderStage
		err     error
		tools   int
		outcome string
		known   bool
	}{
		{"success_response_stream", transport.ProviderStageStream, nil, 1, "completed", true},
		{"success_stage_absent", transport.ProviderStage("UNTRUSTED_PRIVATE_STAGE"), nil, 0, "transport_error", true},
		{"success_earlier_phase_denied", transport.ProviderStageHTTP, nil, 0, "transport_error", true},
		{"authentication_failure", transport.ProviderStageAuthentication, errors.New("synthetic failure"), 0, "transport_error", false},
		{"serialization_failure", transport.ProviderStageSerialization, errors.New("synthetic failure"), 0, "transport_error", false},
		{"http_failure", transport.ProviderStageHTTP, errors.New("synthetic failure"), 0, "transport_error", false},
		{"stream_failure", transport.ProviderStageStream, errors.New("synthetic failure"), 0, "transport_error", false},
		{"unobserved_failure", transport.ProviderStage(""), errors.New("synthetic failure"), 0, "transport_error", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := &releaseChainV6StageSpy{releaseChainV5StreamSpy: &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{event}}, stage: tc.stage, err: tc.err}
			p, live, req := releaseChainV6OwnedProviderFixture(t, spy)
			stream, err := p.StartRun(live, req)
			if (err != nil) != (tc.err != nil) {
				t.Fatal("entry error differs")
			}
			tools := 0
			if stream != nil {
				for out := range stream {
					if out.EventType == transport.EventTypeToolCallRequested {
						tools++
						var durable bool
						if p.window.pool.QueryRow(context.Background(), `SELECT outcome='completed' AND total_tokens=205 AND provider_operation_stage='response_stream' FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=538`).Scan(&durable) != nil || !durable {
							t.Fatal("tool release preceded durable stage and expense")
						}
					}
				}
			}
			var stage *string
			var in, out, total *int64
			var observedVersion *string
			var outcome string
			if p.window.pool.QueryRow(context.Background(), `SELECT provider_operation_stage,input_tokens,output_tokens,total_tokens,model_version,outcome FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=538`).Scan(&stage, &in, &out, &total, &observedVersion, &outcome) != nil {
				t.Fatal("owning measurement unavailable")
			}
			validStage := tc.stage == transport.ProviderStageAuthentication || tc.stage == transport.ProviderStageSerialization || tc.stage == transport.ProviderStageHTTP || tc.stage == transport.ProviderStageStream
			if tools != tc.tools || outcome != tc.outcome || (stage != nil) != validStage || stage != nil && *stage != string(tc.stage) || (in != nil) != tc.known || (out != nil) != tc.known || (total != nil) != tc.known || (observedVersion != nil) != tc.known || spy.calls.Load() != 1 {
				t.Fatal("stage invented, expense guessed, or wrong release", outcome, tools)
			}
			if _, err = p.window.pool.Exec(context.Background(), `UPDATE `+p.window.table("fixture_chain_v5_high_requests")+` SET provider_operation_stage='response_stream' WHERE ordinal=538`); err == nil {
				t.Fatal("terminal stage overwritten")
			}
		})
	}
}

func TestReleaseChainV6ProviderStageNeverInheritedIntegration(t *testing.T) {
	spy := &releaseChainV6StageSpy{releaseChainV5StreamSpy: &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}}, stage: transport.ProviderStageAuthentication, err: errors.New("synthetic failure")}
	p, live, req := releaseChainV6OwnedProviderFixture(t, spy)
	if _, err := p.StartRun(live, req); err == nil {
		t.Fatal("first failure disappeared")
	}
	spy.stage = ""
	if _, err := p.StartRun(live, req); err == nil {
		t.Fatal("second failure disappeared")
	}
	var first, second *string
	if p.window.pool.QueryRow(context.Background(), `SELECT (SELECT provider_operation_stage FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=538),(SELECT provider_operation_stage FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=539)`).Scan(&first, &second) != nil || first == nil || *first != "authentication" || second != nil {
		t.Fatal("previous request stage filled an unknown new request")
	}
}
