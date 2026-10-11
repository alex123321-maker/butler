package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

// The parent acceptance process supplies only disposable PG and private
// fixture paths. This runs the real service bootstrap and scheduled cleanup.
func TestWorkspaceRuntimeAppHarness(t *testing.T) {
	if os.Getenv("BUTLER_WORKSPACE_APP_HARNESS") != "1" {
		t.Skip("private workspace application harness")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := New(ctx)
	if err != nil {
		t.Fatal("actual workspace application bootstrap failed")
	}
	if err = a.Run(ctx); err != nil {
		t.Fatal("actual workspace application failed")
	}
}
