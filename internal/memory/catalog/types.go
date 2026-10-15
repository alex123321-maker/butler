// Package catalog owns memory source identity, revisions and owner decisions.
// Claims and documents are data. Current permissions remain in task/goal stores.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/security/content"
)

var (
	ErrInvalid = errors.New("invalid memory request")
	ErrDenied  = errors.New("current memory access proof required")
	ErrStale   = errors.New("memory source or decision is stale")
	ErrDeleted = errors.New("memory source is tombstoned")
	ErrRestore = errors.New("memory deletion watermark requires reconciliation")
)

type Source struct {
	ID             string          `json:"source_id"`
	OwnerID        string          `json:"owner_id"`
	Kind           string          `json:"source_kind"`
	ScopeType      string          `json:"scope_type"`
	ScopeID        string          `json:"scope_id"`
	ExternalKey    string          `json:"external_key"`
	Revision       int64           `json:"revision"`
	AccessRevision int64           `json:"access_revision"`
	Status         string          `json:"status"`
	Hash           string          `json:"content_hash"`
	Title          string          `json:"title"`
	Body           string          `json:"body"`
	Locator        json.RawMessage `json:"locator"`
	Origin         content.Origin  `json:"origin"`
	Author         string          `json:"author"`
	EventTime      *time.Time      `json:"event_time"`
	ExtractedAt    time.Time       `json:"extracted_at"`
}
type PutSource struct {
	ExternalKey      string          `json:"external_key"`
	Kind             string          `json:"source_kind"`
	ScopeType        string          `json:"scope_type"`
	ScopeID          string          `json:"scope_id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Title            string          `json:"title"`
	Body             string          `json:"body"`
	Locator          json.RawMessage `json:"locator"`
	Author           string          `json:"author"`
	EventTime        *time.Time      `json:"event_time"`
	// Trusted ingest adapters, not an HTTP body, provide provenance/dependencies.
	Origin  content.Origin `json:"-"`
	Parents []SourceRef    `json:"-"`
}
type SourceRef struct {
	ID             string `json:"source_id"`
	Revision       int64  `json:"revision"`
	AccessRevision int64  `json:"access_revision"`
	Hash           string `json:"content_hash"`
}
type Claim struct {
	MemoryType     string          `json:"memory_type"`
	ID             string          `json:"claim_id"`
	OwnerID        string          `json:"owner_id"`
	ScopeType      string          `json:"scope_type"`
	ScopeID        string          `json:"scope_id"`
	Key            string          `json:"key"`
	SourceID       string          `json:"source_id"`
	SourceRevision int64           `json:"source_revision"`
	Value          json.RawMessage `json:"value"`
	Summary        string          `json:"summary"`
	Status         string          `json:"status"`
	Confidence     float64         `json:"extraction_confidence"`
	SupersedesID   string          `json:"supersedes_id"`
	AcceptedBy     string          `json:"accepted_by"`
	EventTime      *time.Time      `json:"event_time"`
	ExtractedAt    time.Time       `json:"extracted_at"`
}
type ClaimRequest struct {
	// Trusted consumer projection; never chosen by a model or HTTP claim body.
	MemoryType     string          `json:"-"`
	SourceID       string          `json:"source_id"`
	SourceRevision int64           `json:"source_revision"`
	Key            string          `json:"key"`
	Value          json.RawMessage `json:"value"`
	Summary        string          `json:"summary"`
	Confidence     float64         `json:"extraction_confidence"`
	// Trusted extractor fingerprint; a model/HTTP body cannot select retry identity.
	DedupKey string `json:"-"`
}
type Decision struct {
	ClaimID          string `json:"claim_id"`
	ExpectedActiveID string `json:"expected_active_id"`
	Reject           bool   `json:"reject"`
}
type Fragment struct {
	Key     string          `json:"key"`
	Title   string          `json:"title"`
	Text    string          `json:"text"`
	Locator json.RawMessage `json:"locator"`
}
type Deletion struct {
	ID        string `json:"deletion_id"`
	SourceID  string `json:"source_id"`
	OwnerID   string `json:"owner_id"`
	Action    string `json:"action"`
	Watermark int64  `json:"watermark"`
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func sourceID(owner, key string) string { return "source_" + hash([]string{owner, key}) }
