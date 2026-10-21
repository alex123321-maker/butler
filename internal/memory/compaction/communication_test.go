package compaction

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/summary"
	"strings"
	"testing"
)

func TestCommunicationCompactionPreservesInternalFinalKind(t *testing.T) {
	// Enough primary text to make exact selection a real compaction.
	d := Document{Version: 1, SessionKey: "session", SourceRevision: 1, ThroughSequence: 2, ThroughMessageID: "final", Events: []Event{{ID: "pub", Sequence: 1, Role: "assistant", Kind: "publication", Text: strings.Repeat("visible message ", 150)}, {ID: "final", Sequence: 2, Role: "system", Kind: "attempt_final", Text: ""}}}
	raw, _ := json.Marshal(d)
	sources := []catalog.Source{{Body: string(raw), ScopeType: "session", ScopeID: "session", Kind: "document"}}
	out, e := (Consumer{}).Validate(sources, `{"events":[{"message_id":"pub","excerpts":["visible message"]},{"message_id":"final","excerpts":[]}]}`)
	if e != nil || !strings.Contains(out, `"kind":"attempt_final"`) {
		t.Fatal("internal final mislabeled by compaction", out, e)
	}
}

func TestCommunicationCompactionReadsAndPublishesTypedFinalIntegration(t *testing.T) {
	f := newConsumerFixture(t)
	if _, e := f.pool.Exec(f.ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES('publication','session','run','assistant',$1,'{"kind":"publication"}'),('final:run','session','run','system','','{"kind":"attempt_final","visibility":"internal","published":false}')`, strings.Repeat("Published owner answer. ", 80)); e != nil {
		t.Fatal(e)
	}
	f.append(t, 4)
	f.poll(t)
	if did, e := f.worker.ProcessOne(f.ctx); e != nil || !did {
		t.Fatal("actual summary worker", did, e)
	}
	snapshot, e := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
	if e != nil || !strings.Contains(snapshot.Summary, `"kind":"attempt_final"`) || !strings.Contains(snapshot.Summary, `"kind":"publication"`) {
		t.Fatal("database kinds lost during actual summary publication", snapshot.Summary, e)
	}
}
