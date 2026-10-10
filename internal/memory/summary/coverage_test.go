package summary

import (
	"github.com/butler/butler/internal/memory/transcript"
	"testing"
)

func TestCoverageRequiresMatchingSourceAndBoundary(t *testing.T) {
	source := transcript.Transcript{SourceRevision: 3, Messages: []transcript.Message{
		{SessionKey: "s", MessageID: "a", Sequence: 1}, {SessionKey: "s", MessageID: "b", Sequence: 2},
	}}
	valid := Snapshot{SessionKey: "s", Summary: "history", ThroughMessageID: "a", ThroughSequence: 1, SourceRevision: 3, FormatVersion: 1}
	for _, name := range []string{"valid", "edited", "future", "wrong_id", "wrong_session", "unversioned", "empty"} {
		t.Run(name, func(t *testing.T) {
			snapshot := valid
			switch name {
			case "edited":
				snapshot.SourceRevision = 2
			case "future":
				snapshot.ThroughSequence = 3
				snapshot.ThroughMessageID = "c"
			case "wrong_id":
				snapshot.ThroughMessageID = "b"
			case "wrong_session":
				snapshot.SessionKey = "other"
			case "unversioned":
				snapshot.FormatVersion = 0
			case "empty":
				snapshot.Summary = ""
			}
			tail, verified := Tail(source, snapshot)
			if name == "valid" {
				if !verified || len(tail) != 1 || tail[0].MessageID != "b" {
					t.Fatal("valid prefix was not separated")
				}
			} else if verified || len(tail) != 2 {
				t.Fatal("unverified coverage discarded source history")
			}
		})
	}
}
