package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type activatedContinuationReader struct {
	value ConverterContinuationContext
	calls int
	deny  bool
}

func (r *activatedContinuationReader) LoadConverterContinuation(context.Context, ingress.InputEvent) (ConverterContinuationContext, error) {
	r.calls++
	if r.deny {
		return ConverterContinuationContext{}, errors.New("current proof revoked")
	}
	return r.value, nil
}
func activatedContextFixture() (*activatedContinuationReader, *Service, context.Context, ingress.InputEvent, preparedRun) {
	e := ingress.InputEvent{Source: "converter-return", EventID: "converter-return:current055", SessionKey: "session", PayloadJSON: `{"task_id":"task","continuation_id":"current055"}`}
	r := &activatedContinuationReader{value: ConverterContinuationContext{RunID: "run", SessionKey: "session", TaskID: "task", ID: "current055", Purpose: generatedtools.ActivatedCatalog, ChildTaskID: "child", Activation: &ConverterContinuationActivation{ConverterID: "inventory", Digest: strings.Repeat("a", 64), Generation: 2, ActionID: "confirmed-activation", EffectID: "confirmed-effect"}}}
	s := &Service{config: Config{ConverterContinuations: r}}
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{RunID: "run", SessionKey: "session"})
	p := preparedRun{InputPayload: map[string]any{"task_id": "task"}, InputItems: []transport.InputItem{content.CurrentOwnerMessage("original owner goal", "input:original"), content.CurrentOwnerMessage("synthetic trigger", "input:"+e.EventID)}}
	return r, s, ctx, e, p
}
func TestConverterContinuationActivatedCatalogTruthAndRecovery(t *testing.T) {
	r, s, ctx, event, p := activatedContextFixture()
	if err := s.attachConverterContinuation(ctx, event, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.InputItems) != 3 || p.InputItems[0].Content != "original owner goal" || !strings.Contains(p.InputItems[1].Content, "already activated") || !strings.Contains(p.InputItems[1].Content, "do not repeat") {
		t.Fatal("activated continuation restarted candidate instructions or lost owner goal")
	}
	var frame struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal([]byte(p.InputItems[2].Content), &frame) != nil || frame.Data["technical_candidate_available"] != false || frame.Data["activated_converter_available"] != true || frame.Data["original_task_verified"] != false || frame.Data["instruction_authority"] != false || frame.Data["confirmed_activation"].(map[string]any)["catalog_generation"] != float64(2) || content.ItemOrigin(p.InputItems[2]).IsInstruction() {
		t.Fatal("actual phase/generation or DATA boundary lost")
	}
	durable := durableSourcePreparation(p)
	if strings.Contains(durable.InputItems[2].Content, "confirmed_activation") {
		t.Fatal("cached activation survived opaque durable checkpoint")
	}
	for range 2 {
		if err := s.rehydrateSourcePreparation(ctx, &durable); err != nil {
			t.Fatal(err)
		}
	}
	if r.calls != 3 || len(durable.InputItems) != 3 || len(durable.SourceDataRefs) != 1 {
		t.Fatal("activation recovery skipped current proof or duplicated facts")
	}
	r.deny = true
	if err := s.rehydrateSourcePreparation(ctx, &durable); err == nil {
		t.Fatal("revoked activation proof reused from cache")
	}
}
func TestConverterContinuationActivatedMalformedProofDenied(t *testing.T) {
	for _, kind := range []string{"missing_activation", "bad_digest", "zero_generation", "missing_receipt", "candidate_activation"} {
		t.Run(kind, func(t *testing.T) {
			r, s, ctx, event, p := activatedContextFixture()
			switch kind {
			case "missing_activation":
				r.value.Activation = nil
			case "bad_digest":
				r.value.Activation.Digest = strings.Repeat("z", 64)
			case "zero_generation":
				r.value.Activation.Generation = 0
			case "missing_receipt":
				r.value.Activation.EffectID = ""
			case "candidate_activation":
				r.value.Purpose = generatedtools.CandidateReturn
			}
			if err := s.attachConverterContinuation(ctx, event, &p); err == nil {
				t.Fatal("wrong activated phase/proof entered model context")
			}
		})
	}
}
