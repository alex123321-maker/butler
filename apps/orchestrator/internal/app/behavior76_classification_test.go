package app

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/ingress"
	"sync/atomic"
	"testing"
	"time"
)

func TestBehavior76LocalUnknownDeliveryConflictIsGlobalIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	f.seed(e)
	base := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	p := behavior76PublicWrapped(t, f, &global, base)
	if outcome, err := f.evaluate(p, e.Steps[0]); err != nil || outcome.State != "completed" {
		t.Fatal(err)
	}
	changed := e.Steps[0]
	changed.Event.Text = "Different payload under the same accepted delivery identity"
	if _, err := f.evaluate(p, changed); !errors.Is(err, input.ErrDeliveryConflict) || !global.Load() {
		t.Fatal("delivery identity corruption became ordinary expected admission denial", err)
	}
	if base.starts != 1 || base.submits != 1 {
		t.Fatal("conflicting delivery replayed the model")
	}
	t.Log("actual changed-delivery conflict globally stops study, preserves original input/result and denies model replay; native/auth=0")
}

func TestBehavior76LocalUnknownPreCoreFailureIsGlobalIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	f.seed(e)
	if _, err := f.pool.Exec(ctx, `DROP TABLE behavior76_task_mandates`); err != nil {
		t.Fatal(err)
	}
	base := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	executor := behavior76TaskExecutor{f: f, provider: base}
	_, err := executor.Execute(ctx, ingress.InputEvent{PayloadJSON: behavior76JSON(map[string]any{"task_id": f.lookup("task", "A")})})
	if err == nil || !global.Load() || base.starts != 0 || base.submits != 0 {
		t.Fatal("pre-Core storage failure was swallowed as a semantic/ordinary blocked case", err)
	}
	t.Log("unavailable pre-Core mandate storage globally seals before native/provider entry; native/auth=0")
}
