package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type parallelRecoveryEntry struct {
	ctx   context.Context
	token ownership.Token
	ref   transport.TransportRunContext
}

// One shared scripted provider holds both real core streams open. The barrier
// changes only response timing; all input claims, leases, sources, budgets,
// Broker admissions and final results come from their production owners.
type parallelRecoveryProvider struct {
	*fullHighPreflightProvider
	entered chan parallelRecoveryEntry
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (p *parallelRecoveryProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	if len(request.ToolDefinitions) != 0 {
		return p.fullHighPreflightProvider.StartRun(ctx, request)
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != request.Context.RunID || p.f.sources.CheckRunSources(ctx, token.RunID) != nil {
		return nil, errNativeFixtureDenied
	}
	stream, err := p.fullHighPreflightProvider.StartRun(ctx, request)
	if err != nil {
		return nil, err
	}
	active := p.active.Add(1)
	for old := p.maximum.Load(); active > old; old = p.maximum.Load() {
		if p.maximum.CompareAndSwap(old, active) {
			break
		}
	}
	select {
	case p.entered <- parallelRecoveryEntry{ctx: ctx, token: token, ref: request.Context}:
	case <-ctx.Done():
		p.active.Add(-1)
		return nil, ctx.Err()
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer p.active.Add(-1)
		select {
		case <-p.release:
		case <-ctx.Done():
			return
		}
		for event := range stream {
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func parallelRecoveryWaitPG(t *testing.T, ctx context.Context, pool *pgxpool.Pool, deadline time.Time) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var now time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		if !now.Before(deadline) {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("natural PostgreSQL claim expiry deadline", ctx.Err())
		}
	}
}

func TestDelegationParallelRecoveryWaitsNaturalClaimExpiryWithoutInferenceIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	admission, broker := nativeDiagnosticBroker(t, f, 2)
	const key = "parallel-recovery-parent"
	if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, key, key), nativeScriptedTaskRequest(f, key))
	if err != nil {
		t.Fatal(err)
	}
	event, err := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: key, SessionKey: key, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": parent.TaskID, "text": fullHighParentPrompt(f.args)}, DeliveryID: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.inputs.Accept(nativeOwner(ctx, key, key), event, nativeFixtureOwner, parent.TaskID); err != nil {
		t.Fatal(err)
	}
	base := &fullHighPreflightProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}}, f: f}
	provider := &parallelRecoveryProvider{fullHighPreflightProvider: base, entered: make(chan parallelRecoveryEntry, 2), release: make(chan struct{})}
	reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
	core := nativeDiagnosisCore(f, provider, broker, reader, f.inputs)
	claim, err := f.inputs.Claim(ctx, time.Minute)
	if err != nil || claim.TaskID != parent.TaskID {
		t.Fatal("actual parent claim unavailable", err)
	}
	parentEvent := claim.Event
	parentEvent.IdempotencyKey = claim.InputID
	parentFinal, err := core.Execute(durableinput.WithClaim(ctx, claim), parentEvent)
	if err != nil || parentFinal == nil {
		t.Fatal("actual parent/Broker admission failed", err)
	}
	if err := f.inputs.Finish(ctx, claim, "completed", "", "", 0); err != nil {
		t.Fatal(err)
	}
	parentEvidence := &fullHighWidthAcceptance{ResolutionPolicy: "owner_selected_source_a_for_fixture"}
	checkFullHighParentReceipts(t, f, parent.TaskID, parentFinal.RunID, parentEvidence)

	// Simulate two lost workers using only the owning claim API. Equal short TTLs
	// are intentionally started at different PG times; no claim/lease timestamp
	// is edited and neither child has entered a provider or acquired a run yet.
	first, err := f.inputs.Claim(ctx, 3*time.Second)
	if err != nil || first.ClaimExpiresAt == nil || first.TaskID == parent.TaskID {
		t.Fatal("first actual child claim unavailable", err)
	}
	parallelRecoveryWaitPG(t, ctx, f.pool, first.ClaimExpiresAt.Add(-2*time.Second))
	second, err := f.inputs.Claim(ctx, 3*time.Second)
	if err != nil || second.ClaimExpiresAt == nil || second.TaskID == first.TaskID || second.SessionKey == first.SessionKey || second.ClaimExpiresAt.Sub(*first.ClaimExpiresAt) < 800*time.Millisecond {
		t.Fatal("child claims were not independent and staggered", err)
	}
	parallelRecoveryWaitPG(t, ctx, f.pool, *first.ClaimExpiresAt)
	var firstExpired, secondLive bool
	if err := f.pool.QueryRow(ctx, `SELECT a.claim_expires_at<=clock_timestamp(),b.claim_expires_at>clock_timestamp() FROM accepted_inputs a,accepted_inputs b WHERE a.input_id=$1 AND b.input_id=$2`, first.InputID, second.InputID).Scan(&firstExpired, &secondLive); err != nil || !firstExpired || !secondLive || provider.childEntries.Load() != 0 {
		t.Fatal("natural staggered expiry window was not observed", err)
	}
	parallelRecoveryWaitPG(t, ctx, f.pool, *second.ClaimExpiresAt)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
	worker := durableinput.NewService(f.inputs, f.sessions, core, runs, log)
	worker.SetMaintenance((&delegationMaintenance{pool: f.pool, manager: admission.Manager}).Sweep)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerCtx) }()
	defer func() { stop(); <-done }()
	entries := make([]parallelRecoveryEntry, 0, 2)
	for range 2 {
		select {
		case entry := <-provider.entered:
			entries = append(entries, entry)
		case <-ctx.Done():
			t.Fatal("default two workers did not overlap actual child core streams")
		}
	}
	if provider.active.Load() != 2 || provider.maximum.Load() != 2 || entries[0].ref.RunID == entries[1].ref.RunID || entries[0].ref.SessionKey == entries[1].ref.SessionKey || entries[0].token.LeaseID == entries[1].token.LeaseID {
		t.Fatal("parallel recovery did not preserve two separate current contexts")
	}
	for _, prior := range []durableinput.Record{first, second} {
		current, err := f.inputs.Get(ctx, prior.InputID)
		if err != nil || current.Event != prior.Event || current.TaskID != prior.TaskID || current.SessionKey != prior.SessionKey || current.Attempts != 2 || current.RunID == "" {
			t.Fatal("recovery changed immutable accepted child identity or refs", err)
		}
	}
	for _, entry := range entries {
		if err := f.sources.CheckRunSources(entry.ctx, entry.ref.RunID); err != nil {
			t.Fatal("overlapping child lost its current source/fence proof", err)
		}
		var current bool
		if err := f.pool.QueryRow(ctx, `SELECT i.attempts=2 AND i.claim_id<>$2 AND i.claim_id<>$3 AND i.status='running' AND i.claim_expires_at>clock_timestamp() AND o.lease_id=$4 AND o.fence_epoch=$5 AND o.expires_at>clock_timestamp() AND r.task_revision=t.revision AND d.state='executing' FROM runs r JOIN tasks t USING(task_id) JOIN task_delegations d USING(task_id) JOIN accepted_inputs i ON i.run_id=r.run_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1`, entry.ref.RunID, first.ClaimID, second.ClaimID, entry.token.LeaseID, entry.token.FenceEpoch).Scan(&current); err != nil || !current {
			t.Fatal("child entry did not own actual reclaimed input/current task lease", err)
		}
	}
	close(provider.release)
	nativeWait(t, ctx, 15*time.Second, func() bool {
		var n int
		return f.pool.QueryRow(ctx, `SELECT count(*) FROM task_delegations d JOIN accepted_inputs i ON i.task_id=d.task_id WHERE d.parent_task_id=$1 AND d.state='result_ready' AND i.status='completed'`, parent.TaskID).Scan(&n) == nil && n == 2
	})
	for _, entry := range entries {
		final, err := completion.New(f.pool).GetByRun(ctx, entry.ref.RunID)
		if err != nil || final.Verdict != "unverified" || final.PrivacyRedacted {
			t.Fatal("parallel recovery promoted or lost actual child final", err)
		}
		var raw []byte
		if err := f.pool.QueryRow(ctx, `SELECT contract_json FROM task_delegations WHERE task_id=$1`, final.TaskID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var contract delegation.Contract
		if err := json.Unmarshal(raw, &contract); err != nil || len(contract.Sources) != 1 {
			t.Fatal("actual immutable child contract unavailable", err)
		}
		original, err := f.sources.Get(nativeOwner(ctx, "parallel-original-"+final.TaskID, ""), contract.Sources[0].ID)
		if err != nil || guardSource(original) != contract.Sources[0] || delegation.VerifyNumericObservation([]byte(final.Content), contract.Sources[0], []byte(original.Body)) != nil {
			t.Fatal("parallel scripted final failed current original pins", err)
		}
	}
	var spent, effects int
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 27 || provider.parentStarts.Load() != 1 || provider.continuations.Load() != 2 || provider.childEntries.Load() != 2 {
		t.Fatal("parallel preflight charged native inference or replayed parent/child")
	}
	if f.pool.QueryRow(ctx, `SELECT count(DISTINCT e.action_id) FROM action_effects e JOIN action_intents a ON a.effect_id=e.action_id JOIN runs r ON r.run_id=a.run_id WHERE r.task_id=$1 AND e.state='effect_confirmed'`, parent.TaskID).Scan(&effects) != nil || effects != 2 {
		t.Fatal("parallel recovery lost actual Broker admission effects")
	}
}
