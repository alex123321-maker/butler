package broker

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type deadlineContext struct {
	*authFixture
	deadline time.Time
}

func (r deadlineContext) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	c, err := r.authFixture.GetActionContext(ctx, id)
	c.AuthorityDeadline = r.deadline
	return c, err
}

type deadlineExecutor struct {
	deadline time.Time
	bounded  bool
}

func (e *deadlineExecutor) Execute(ctx context.Context, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	e.deadline, e.bounded = ctx.Deadline()
	return &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{}`}, nil
}
func TestRuntimeCallCannotOutliveKnownOwnerDeadline(t *testing.T) {
	f := newAuthFixture()
	d := time.Now().Add(30 * time.Second)
	executor := &deadlineExecutor{}
	s := NewServer(loadTestRegistry(t), executor, nil, nil)
	s.SetActionAuthorizer(actionauth.NewService(f, deadlineContext{f, d}, nil))
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "deadline-call", ToolName: "http.request", ArgsJson: `{"method":"POST","url":"https://example.com/test-service"}`}
	result := dispatchForTest(t, s, f, call, true)
	// The synthetic POST has no external verification receipt. Receiving a
	// runtime reply proves dispatch only, so the journal must remain uncertain.
	if !executor.bounded || !executor.deadline.Equal(d) || result.GetEffectState() != string(actionauth.StateUnknown) {
		t.Fatal("runtime release exceeded exact owner bound", executor, result)
	}
}
