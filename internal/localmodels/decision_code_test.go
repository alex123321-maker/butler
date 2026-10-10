package localmodels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func TestDecisionCodeIsExactAndFailClosedBeforeTrustLoader(t *testing.T) {
	b := processFixture(t, "exit 0\n")
	p := &pb.ModelProfile{Model: "LiquidAI/d1-omni-600M", Revision: DecisionRevision, Runtime: "torch-d1-reviewed", RuntimeVersion: "torch2.10.0+cu128/transformers5.19.0/d1code02b55d7", Format: "safetensors", Precision: "FP32", SourceUrl: "https://huggingface.co/LiquidAI/d1-omni-600M", PreferredDevice: "cpu", PreprocessingVersion: "d1-text-choice-v1"}
	root := filepath.Join(b.cfg.ModelRoot, DecisionRevision)
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]string{"model": p.Model, "revision": p.Revision, "runtime_version": p.RuntimeVersion})
	if e := os.WriteFile(filepath.Join(root, "butler-model.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := b.Verify(context.Background(), p); !isFault(e, Unavailable) {
		t.Fatal("missing code accepted", e)
	}
	for name := range decisionCode {
		if e := os.WriteFile(filepath.Join(root, name), []byte("unreviewed replacement"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := b.Verify(context.Background(), p); !isFault(e, Unavailable) || b.cmd != nil {
		t.Fatal("unreviewed code reached runtime", e)
	}
	p.Revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if e := b.verifyDecisionCode(p); !isFault(e, Unavailable) {
		t.Fatal("new revision trusted", e)
	}
}

func TestClassifierInputLabelsRemainBoundedAndUnique(t *testing.T) {
	p := profile()
	p.Capabilities = []pb.Capability{pb.Capability_CAPABILITY_CLASSIFY}
	for _, labels := range [][]string{{"stop", "stop"}, {"stop", string(make([]byte, 129))}} {
		r := &pb.InferRequest{RequestId: "test", ProfileId: p.Id, Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "event", Labels: labels}}}
		if e := validateRequest(p, r); !isFault(e, Invalid) {
			t.Fatal("ambiguous/unbounded labels accepted", e)
		}
	}
}
