package input

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

type claimWaitingExecutor struct {
	start func(context.Context) error
	cause error
}

func (e *claimWaitingExecutor) Execute(ctx context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
	if e.start != nil {
		if err := e.start(ctx); err != nil {
			return nil, err
		}
	}
	<-ctx.Done()
	e.cause = context.Cause(ctx)
	return nil, ctx.Err()
}

func TestCommunicationInputClaimCancellationIntegration(t *testing.T) {
	for _, failure := range []string{"replaced", "storage"} {
		t.Run(failure, func(t *testing.T) {
			dsn := os.Getenv("BUTLER_TEST_COMMUNICATION_POSTGRES_URL")
			if dsn == "" {
				t.Skip("isolated communication PostgreSQL required")
			}
			t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if _, _, err := session.NewPostgresRepository(pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: "web:claim", UserID: "owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
				t.Fatal(err)
			}
			store := NewStore(pool)
			if _, _, err := store.Accept(ctx, ingress.InputEvent{SessionKey: "web:claim", EventID: "claim-input", IdempotencyKey: "claim-input", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"one"}`, CreatedAt: time.Now()}, "owner", ""); err != nil {
				t.Fatal(err)
			}
			claim, err := store.Claim(ctx, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			executor := &claimWaitingExecutor{}
			if failure == "storage" {
				_, err = pool.Exec(ctx, `CREATE FUNCTION reject_fixture_renew() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN IF NEW.claim_expires_at IS NOT NULL THEN RAISE EXCEPTION 'private renewal database/source sentinel'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_fixture_renew BEFORE UPDATE OF claim_expires_at ON accepted_inputs FOR EACH ROW EXECUTE FUNCTION reject_fixture_renew()`)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				executor.start = func(ctx context.Context) error {
					_, err := pool.Exec(ctx, `UPDATE accepted_inputs SET claim_id='new-owner-claim' WHERE input_id=$1`, claim.InputID)
					return err
				}
			}
			var logs bytes.Buffer
			worker := NewService(store, nil, executor, nil, slog.New(slog.NewTextHandler(&logs, nil)))
			worker.claimTTL = 60 * time.Millisecond
			worker.process(ctx, claim)
			want := "input_claim_renewal_failed"
			if failure == "replaced" {
				want = "input_claim_lost"
			}
			if executor.cause == nil || executor.cause.Error() != want || strings.Contains(logs.String(), "sentinel") {
				t.Fatal("claim cause lost or leaked storage details", executor.cause)
			}
			row, err := store.Get(ctx, claim.InputID)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "storage" {
				if row.Status != "blocked" || row.ErrorCode != want || row.ErrorMessage != want {
					t.Fatal("durable claim failure category lost", row.Status, row.ErrorCode, row.ErrorMessage)
				}
			} else if row.ClaimID != "new-owner-claim" || row.Status != "running" {
				t.Fatal("old worker overwrote its replacement claim")
			}
		})
	}
}

func TestCommunicationClaimCausePreservesIndependentFailures(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(&claimCancellation{lost: true})
	provider := &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Message: "provider_unavailable"}
	for _, err := range []error{provider, errors.Join(provider, context.Canceled), errors.Join(context.DeadlineExceeded, context.Canceled), diagnosedCancellation{}} {
		if preserveClaimCancellation(ctx, err) != err {
			t.Fatal("independent failure relabelled")
		}
	}
	if err := preserveClaimCancellation(ctx, context.Canceled); err.Error() != "input_claim_lost" || !errors.Is(err, ErrClaimLost) {
		t.Fatal("claim cancellation not retained")
	}
}

type diagnosedCancellation struct{}

func (diagnosedCancellation) Error() string          { return "execution_ownership_changed" }
func (diagnosedCancellation) DiagnosticCode() string { return "execution_ownership_changed" }
func (diagnosedCancellation) Unwrap() error          { return context.Canceled }
