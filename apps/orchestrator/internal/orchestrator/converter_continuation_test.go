package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type continuationContextFixture struct {
	deny  bool
	calls int
}

func (f *continuationContextFixture) LoadConverterContinuation(_ context.Context, e ingress.InputEvent) (ConverterContinuationContext, error) {
	f.calls++
	if f.deny {
		return ConverterContinuationContext{}, errors.New("current proof revoked")
	}
	return ConverterContinuationContext{RunID: "run", SessionKey: e.SessionKey, TaskID: "task", ID: "current055", Purpose: generatedtools.CandidateReturn, ChildTaskID: "child"}, nil
}
func TestConverterContinuationRecoveryKeepsDataAndRequiresCurrentProof(t *testing.T) {
	event := ingress.InputEvent{Source: "converter-return", EventID: "converter-return:current055", SessionKey: "session", PayloadJSON: `{"task_id":"task","continuation_id":"current055"}`}
	reader := &continuationContextFixture{}
	service := &Service{config: Config{ConverterContinuations: reader}}
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{RunID: "run", SessionKey: "session"})
	prepared := preparedRun{InputPayload: map[string]any{"task_id": "task"}, InputItems: []transport.InputItem{content.CurrentOwnerMessage("original owner goal", "input:original"), content.CurrentOwnerMessage("synthetic receipt must not be owner authority", "input:"+event.EventID)}}
	if err := service.attachConverterContinuation(ctx, event, &prepared); err != nil {
		t.Fatal(err)
	}
	if len(prepared.InputItems) != 3 || prepared.InputItems[0].Content != "original owner goal" {
		t.Fatal("original authority lost or synthetic owner retained")
	}
	if content.ItemOrigin(prepared.InputItems[1]).Kind != content.KernelInstruction || content.ItemOrigin(prepared.InputItems[2]).Kind != content.SubagentOutput || content.ItemOrigin(prepared.InputItems[2]).IsInstruction() {
		t.Fatal("technical readiness promoted to owner instruction")
	}
	durable := durableSourcePreparation(prepared)
	if strings.Contains(durable.InputItems[2].Content, "technical_candidate_available") {
		t.Fatal("cached readiness retained instead of opaque projection")
	}
	for range 2 {
		if err := service.rehydrateSourcePreparation(ctx, &durable); err != nil {
			t.Fatal(err)
		}
	}
	if len(durable.InputItems) != 3 || len(durable.SourceDataRefs) != 1 || reader.calls != 3 {
		t.Fatal("recovery reused stale proof or duplicated data")
	}
	reader.deny = true
	if err := service.rehydrateSourcePreparation(ctx, &durable); err == nil {
		t.Fatal("revoked continuation rehydrated cached readiness")
	}
	if err := validateSourceCheckpoint(event, preparedRun{}); err == nil {
		t.Fatal("recovery without opaque receipt accepted")
	}
	replacement := event
	replacement.PayloadJSON = `{"task_id":"foreign","continuation_id":"foreign"}`
	if err := validateSourceCheckpoint(replacement, durable); err == nil {
		t.Fatal("replaced receipt accepted")
	}
}
