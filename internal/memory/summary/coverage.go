package summary

import (
	"strings"

	"github.com/butler/butler/internal/memory/transcript"
)

// Tail validates the coverage against the same source snapshot that supplies
// the tail. An unknown, edited or future boundary never suppresses history.
func Tail(source transcript.Transcript, snapshot Snapshot) ([]transcript.Message, bool) {
	if snapshot.FormatVersion != 1 || snapshot.SourceRevision <= 0 ||
		snapshot.SourceRevision != source.SourceRevision || strings.TrimSpace(snapshot.Summary) == "" {
		return source.Messages, false
	}
	for i, message := range source.Messages {
		if message.MessageID == snapshot.ThroughMessageID && message.Sequence == snapshot.ThroughSequence &&
			message.SessionKey == snapshot.SessionKey {
			return source.Messages[i+1:], true
		}
	}
	return source.Messages, false
}
