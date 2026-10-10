// Package extraction publishes source-anchored candidate data, never mandates,
// permissions or automatically accepted profile entries.
package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/content"
)

const (
	Profile    = "gliner2.5-multi-cpu-v1"
	Model      = "fastino/gliner2.5-multi-v1"
	Revision   = "cf5593a5d45e3bbf204b9df621b13b1c0cf25ee3"
	Schema     = "butler-memory-v1"
	Normalizer = "butler-russian-time-cues-v1"
	Runtime    = "torch2.10.0+cu128/transformers5.19.0/gliner2.1.0"
)

var ErrUnavailable = errors.New("structured memory extraction unavailable")
var ErrInvalid = errors.New("invalid structured memory candidate")

type Backend interface {
	Describe(context.Context, *pb.Empty) (*pb.Catalog, error)
	Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
}
type Field struct {
	Type   string  `json:"type"`
	Text   string  `json:"text"`
	Start  uint32  `json:"start"`
	End    uint32  `json:"end"`
	Score  float64 `json:"extraction_score"`
	Method string  `json:"evidence_method"`
}
type TimeValue struct {
	Raw    string `json:"raw"`
	Status string `json:"status"`
	Date   string `json:"date,omitempty"`
	Zone   string `json:"timezone,omitempty"`
	Basis  string `json:"basis,omitempty"`
}
type Cues struct {
	Scope       string `json:"scope"`
	Negated     bool   `json:"negated"`
	Conditional bool   `json:"conditional"`
	Historical  bool   `json:"historical"`
	Exception   bool   `json:"exception"`
	// Only literal cues; these flags never grant or revoke action authority.
	Evidence []Field `json:"evidence,omitempty"`
}
type Record struct {
	Kind           string          `json:"kind"`
	Fields         []Field         `json:"fields"`
	Unresolved     []string        `json:"unresolved_fields,omitempty"`
	Quote          string          `json:"quote"`
	QuoteStart     uint32          `json:"quote_start"`
	QuoteEnd       uint32          `json:"quote_end"`
	QuoteClipped   bool            `json:"quote_clipped"`
	Cues           Cues            `json:"literal_cues"`
	Times          []TimeValue     `json:"times,omitempty"`
	SourceID       string          `json:"source_id"`
	SourceRevision int64           `json:"source_revision"`
	SourceHash     string          `json:"source_hash"`
	SourceKind     string          `json:"source_kind"`
	Origin         content.Origin  `json:"origin"`
	SourceAuthor   string          `json:"source_author,omitempty"`
	SourceLocator  json.RawMessage `json:"source_locator"`
	EventTime      *time.Time      `json:"event_time,omitempty"`
	IngestedAt     time.Time       `json:"ingested_at"`
	ExtractedAt    time.Time       `json:"extracted_at"`
	Model          string          `json:"model"`
	Revision       string          `json:"model_revision"`
	Runtime        string          `json:"runtime"`
	Schema         string          `json:"schema_version"`
	Normalizer     string          `json:"normalizer_version"`
	Score          float64         `json:"extraction_score"`
	CandidateOnly  bool            `json:"candidate_only"`
}
