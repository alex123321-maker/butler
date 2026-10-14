package localmodels

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func TestFullScoreWorkerRequiresCompleteCorrelatedLabels(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		valid          bool
	}{
		{"complete", `{"kind":"classes","labels":["stop","clarify","task"],"scores":[0.1,0.8,0.1]}`, true},
		{"partial", `{"kind":"classes","labels":["stop","clarify"],"scores":[0.1,0.8]}`, false},
		{"duplicate", `{"kind":"classes","labels":["stop","stop","task"],"scores":[0.1,0.8,0.1]}`, false},
		{"foreign", `{"kind":"classes","labels":["stop","permission","task"],"scores":[0.1,0.8,0.1]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := processFixture(t, fmt.Sprintf("read init\nprintf '%%s\\n' '{\"kind\":\"ready\",\"vram_mib\":0}'\nread request\nprintf '%%s\\n' '%s'\n", tc.response))
			p := &pb.ModelProfile{Id: "full-scores", Model: "fastino/gliner2.5-multi-v1", PreprocessingVersion: GLiNERFullScores}
			t.Cleanup(func() { _ = b.Unload(context.Background(), p.Model) })
			r := &pb.InferRequest{Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "event", Labels: []string{"stop", "clarify", "task"}}}}
			out, err := b.extract(context.Background(), p, r, "cpu")
			if tc.valid {
				if err != nil || len(out.GetClassify().Scores) != 3 {
					t.Fatal("complete worker output rejected", err)
				}
			} else if !isFault(err, Protocol) {
				t.Fatal("malformed alternatives accepted", err)
			}
		})
	}
}
