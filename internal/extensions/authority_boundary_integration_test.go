package extensions

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

type heldContractGuard struct {
	store   *Store
	locked  chan struct{}
	release chan struct{}
}

func (g heldContractGuard) CheckActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	if err := g.store.CheckActionTx(ctx, tx, b); err != nil {
		return err
	}
	close(g.locked)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestExtensionDisableLinearizesWithGrantClaimIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	r := readyFixture(t, s, configuredFixture(t, s, p))
	grantFixtureAccess(t, s, r)
	reader := &actionReader{current: currentContext()}
	store := actionauth.NewPostgresStore(pool)
	guard := heldContractGuard{store: s, locked: make(chan struct{}), release: make(chan struct{})}
	store.SetContractGuard(guard)
	auth := actionauth.NewService(store, reader, nil)
	auth.SetCapabilities(s)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "read", ToolName: "extension.notes.read", ArgsJson: `{"url":"https://extension.example/notes/item/one"}`, AutonomyMode: commonv1.AutonomyMode(3)}
	contract, err := s.GetContract(context.Background(), call.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	grant := approvedGrant(t, auth, reader, call, contract)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	claimed := make(chan error, 1)
	go func() { claimed <- store.ClaimGrant(ctx, grant.ID) }()
	select {
	case <-guard.locked:
	case err := <-claimed:
		t.Fatalf("claim failed before current installation lock: %v", err)
	case <-ctx.Done():
		t.Fatal("claim did not hold current installation")
	}
	disabled := make(chan error, 1)
	go func() { _, err := s.SetEnabled(ownerContext(), r.ID, r.Generation, false); disabled <- err }()
	select {
	case err := <-disabled:
		t.Fatalf("disable crossed the held claim boundary: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(guard.release)
	if err = <-claimed; err != nil {
		t.Fatal(err)
	}
	if err = <-disabled; err != nil {
		t.Fatal(err)
	}
	effect, err := store.GetEffect(ctx, grant.Binding.ActionID)
	if err != nil || effect.State != actionauth.StateExecuting {
		t.Fatal("a previously admitted effect was falsified by later disable")
	}
	// The opposite order is covered by the saved-grant disable test: no
	// executing journal/dispatch exists when disable wins before ClaimGrant.
}
