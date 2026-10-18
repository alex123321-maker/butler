package readcontrol

import (
	"strings"
	"testing"
)

func TestMessageContractsAndBounds(t *testing.T) {
	valid := `{"message_id":"` + strings.Repeat("a", 64) + `","source_id":"source_test","source_revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("b", 64) + `","disposition":"remember","note":"Барбер: четверг 15:00; источник подтверждает запись."}`
	for _, tc := range []struct {
		tool, raw string
		ok        bool
	}{
		{MessageInbox, `{}`, true}, {MessageInbox, `{"query":"escape"}`, false},
		{MessageSearch, `{"query":"барбер","package":"example.test","event_from":"2026-10-01T00:00:00Z"}`, true},
		{MessageSearch, `{"limit":11}`, false}, {MessageSearch, `{"limit":0}`, false}, {MessageSearch, `{"query":"a","query":"b"}`, false},
		{MessageSearch, `{"event_from":"2026-10-02T00:00:00Z","event_until":"2026-10-01T00:00:00Z"}`, false},
		{MessageProcess, valid, true}, {MessageProcess, strings.Replace(valid, `"remember"`, `"grant"`, 1), false},
	} {
		_, err := DecodeArgs(tc.tool, tc.raw)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: valid=%v err=%v", tc.tool, tc.ok, err)
		}
	}
	for _, name := range []string{MessageInbox, MessageSearch, MessageProcess} {
		c := Contract(name)
		if c == nil || c.ToolName != name || c.ContractRevision != "1" {
			t.Fatal(name)
		}
	}
}
