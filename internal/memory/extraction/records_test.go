package extraction

import (
	"encoding/json"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestUnresolvedFragmentPreservesRestrictionWhenModelDropsRecord(t *testing.T) {
	text := "До конца месяца не обновляй рабочий сервер, тестовый можно"
	out := &pb.ExtractOutput{SchemaVersion: Schema, Spans: []*pb.Span{
		span(text, "action", "обновляй"), span(text, "resource", "рабочий сервер"), span(text, "deadline", "До конца месяца"),
	}}
	values := records(catalog.Source{Body: text}, out, 0, "UTC")
	var claims []catalog.Claim
	for _, r := range values {
		if r.Kind == "constraint_fragment" {
			if len(r.Fields) != 3 || len(r.Unresolved) != 2 || r.Unresolved[0] != "action_polarity" || r.Unresolved[1] != "exception_scope" {
				t.Fatal("literal cues invented model record")
			}
			for _, f := range r.Fields {
				if f.Method != "model_span" {
					t.Fatal("model/rule evidence merged")
				}
			}
			for _, f := range r.Cues.Evidence {
				if f.Method != "literal_rule" || f.Score != 0 {
					t.Fatal("literal rule became model score")
				}
			}
		}
		raw, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		claims = append(claims, catalog.Claim{Status: "candidate", Value: raw})
	}
	if !protectedAcceptance(claims) {
		t.Fatal("source restriction lost without model record")
	}
}
