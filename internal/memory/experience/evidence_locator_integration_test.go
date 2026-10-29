package experience

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	"testing"
)

func TestExperienceEvidenceLocatorExactRangesAndUnknownIntegration(t *testing.T) {
	f := newLessonFixture(t)
	lesson := f.publish(t)
	f.accept(t, lesson)
	live, _ := f.consumer(t)
	p, err := f.s.Prepare(live, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(live)
	ss, err := f.s.Sources.AccessTx(live, tx, []catalog.SourceRef{lesson}, "memory_read")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := f.s.EligibleTx(live, tx, ss[0], p, true); err != nil || !ok {
		t.Fatal(err)
	}
	loc, err := f.s.EvidenceLocatorTx(live, tx, ss[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Value EvidenceLocator `json:"experience_evidence_v1"`
	}
	if json.Unmarshal(loc, &decoded) != nil {
		t.Fatal("locator")
	}
	if len(decoded.Value.Ranges) != 6 || decoded.Value.HistoricalTaskVerified {
		t.Fatal("unverified lesson became task proof", decoded)
	}
	r := reader.Reader{Sources: f.s.Sources}
	for _, part := range decoded.Value.Ranges {
		got, err := r.ReadTx(live, tx, readcontrol.Open, readcontrol.Args{SourceID: lesson.ID, Revision: lesson.Revision, AccessRevision: lesson.AccessRevision, Hash: lesson.Hash, Offset: part.Offset, MaxChars: part.MaxChars})
		if err != nil || got.Status != "ok" || len(got.Items) != 1 {
			t.Fatal("pointed evidence not readable", err, part.Field)
		}
		expected := string([]rune(ss[0].Body)[part.Offset : part.Offset+part.MaxChars])
		if got.Items[0].Text != expected {
			t.Fatal("range changed source text", part.Field)
		}
		if !json.Valid([]byte(got.Items[0].Text)) && part.TotalChars <= 8192 {
			t.Fatal("field boundary split JSON", part.Field)
		}
	}
	// Byte offsets and Unicode character offsets are deliberately different.
	body := `{"model_notes":{"summary":"Японский 日本"},"observation":{"owner_verdict":"unknown"}}`
	start, end, ok := jsonRange(body, []string{"observation", "owner_verdict"}, 0)
	if !ok || body[start:end] != `"unknown"` {
		t.Fatal("unicode nested range")
	}
}
