package delegation

import (
	"encoding/hex"
	"strings"
	"time"
)

// InputRequest is a kernel-only ordered child handoff. It contains references,
// never parent transcript, model prose, authority claims or credential material.
type InputRequest struct {
	OwnerID, SessionKey, TaskID, EventID string
	ContractSource                       SourceRef
	Deadline                             time.Time
	MaxAttempts                          int
}

func ValidSource(r SourceRef) bool {
	hash, err := hex.DecodeString(r.Hash)
	return r.ID != "" && len(r.ID) <= 512 && !strings.ContainsAny(r.ID, "\x00\r\n") && r.Revision > 0 && r.AccessRevision > 0 && err == nil && len(hash) == 32 && hex.EncodeToString(hash) == r.Hash
}
