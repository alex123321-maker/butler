package task

import "testing"

func TestRestrictiveOwnerTextRequiresExplicitCommand(t *testing.T) {
	for _, test := range []struct{ text, kind, policy string }{
		{"стоп", ControlStop, ""}, {"СТОП, только подготовь письмо", ControlAmend, PolicyPrepareOnly},
		{"не отправляй письмо, сохрани черновик", ControlAmend, PolicyPrepareOnly}, {"/pause", ControlPause, ""}, {"resume", ControlResume, ""},
		{"What does 'draft only' mean?", "", ""}, {"Объясни фразу не отправляй", "", ""}, {"stopwatch results", "", ""}, {"напиши письмо", "", ""},
	} {
		kind, policy := ClassifyOwnerText(test.text)
		if kind != test.kind || policy != test.policy {
			t.Fatalf("%q => %s/%s", test.text, kind, policy)
		}
	}
}
