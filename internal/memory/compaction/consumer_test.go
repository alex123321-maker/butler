package compaction

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"strings"
	"testing"
)

func modelSelection(d Document) string {
	v := selectionOutput{}
	for _, e := range d.Events {
		snippets := []string{}
		if e.Role == "user" {
			snippets = []string{e.Text}
		} else if len(e.Text) > 30 {
			snippets = []string{e.Text[:30]}
		}
		v.Events = append(v.Events, Selection{ID: e.ID, Excerpts: snippets})
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func TestExtractiveSummaryProtectsEveryOwnerStatement(t *testing.T) {
	d := Document{Version: 1, SessionKey: "s", SourceRevision: 1, ThroughSequence: 2, ThroughMessageID: "b", Events: []Event{{ID: "a", Sequence: 1, Role: "user", Text: "Останови только отправку. Подготовку продолжай. Я уже проверил файл."}, {ID: "b", Sequence: 2, Role: "assistant", Text: strings.Repeat("Working observations; unknown effect. ", 50)}}, Tools: []ToolState{{ID: "mail", Name: "mail.send", Status: "unknown"}}}
	raw, _ := json.Marshal(d)
	src := []catalog.Source{{Kind: "document", ScopeType: "session", ScopeID: "s", Body: string(raw)}}
	out, err := (Consumer{}).Validate(src, modelSelection(d))
	if err != nil || !strings.Contains(out, "Останови только отправку") || !strings.Contains(out, `"observed_status":"unknown"`) {
		t.Fatal(out, err)
	}
	for _, bad := range []string{
		`{"events":[{"message_id":"a","excerpts":["Останови только отправку."]},{"message_id":"b","excerpts":[]}]}`,
		`{"events":[{"message_id":"a","excerpts":["Останови только отправку. Подготовку продолжай. Я уже проверил файл."]},{"message_id":"b","excerpts":["mail successfully sent"]}]}`,
		`{"events":[{"message_id":"a","excerpts":["Останови только отправку. Подготовку продолжай. Я уже проверил файл."]}]}`,
		`{"events":[],"events":[]}`,
	} {
		if _, err = (Consumer{}).Validate(src, bad); err == nil {
			t.Fatalf("unsafe selection accepted: %s", bad)
		}
	}
}
