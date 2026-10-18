// Package semantic owns versioned retrieval indexes, never memory truth or authority.
package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/security/content"
)

var (
	ErrInvalid     = errors.New("invalid semantic index input")
	ErrSourceLimit = fmt.Errorf("semantic scope exceeds the finite chunk limit: %w", ErrInvalid)
	ErrUnavailable = errors.New("semantic retrieval unavailable")
	ErrStale       = fmt.Errorf("semantic source or activation changed: %w", catalog.ErrStale)
	ErrUnverified  = errors.New("semantic generation not verified")
	ErrAdmission   = errors.New("semantic inference admission required")
	ErrAccess      = fmt.Errorf("semantic source access denied: %w", catalog.ErrDenied)
)

// Space is immutable. Its digest distinguishes incompatible vectors even when
// their dimension happens to match. Query/code and document modes are versioned
// by Preprocessing; no vector is written into a legacy memory column.
type Space struct {
	ProfileID      string `json:"profile_id"`
	Model          string `json:"model"`
	Revision       string `json:"revision"`
	Preprocessing  string `json:"preprocessing"`
	RuntimeVersion string `json:"runtime_version"`
	ChunkVersion   string `json:"chunk_version"`
	Dimensions     int    `json:"dimensions"`
}

func (s Space) Validate() error {
	if s.ProfileID == "" || s.Model != "google/embeddinggemma-2" || len(s.Revision) != 40 || s.Preprocessing != "embeddinggemma2-retrieval-v1" || s.RuntimeVersion == "" || s.ChunkVersion == "" || len(s.ChunkVersion) > 128 || s.Dimensions != 256 && s.Dimensions != 768 {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(s.Revision); err != nil {
		return ErrInvalid
	}
	return nil
}
func (s Space) ID() string {
	raw, _ := json.Marshal(s)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
func (s Space) WireID() string {
	return s.ProfileID + ":" + s.Revision + ":" + s.Preprocessing + ":" + strconv.Itoa(s.Dimensions)
}

type Scope struct{ Type, ID string }

func (s Scope) valid() bool {
	return (s.Type == "session" || s.Type == "user" || s.Type == "global" || s.Type == "project" || s.Type == "task") && strings.TrimSpace(s.ID) != "" && len(s.ID) <= 256
}

type Source struct {
	ID                                                            int64
	Scope                                                         Scope
	Title, Content, Summary, SourceType, SourceID, ProvenanceJSON string
	Revision                                                      time.Time
	CatalogSourceID                                               string
	CatalogRevision, AccessRevision                               int64
}

func (s *Source) catalogMetadata() {
	var metadata struct {
		Catalog struct {
			SourceID       string `json:"source_id"`
			Revision       int64  `json:"revision"`
			AccessRevision int64  `json:"access_revision"`
		} `json:"catalog"`
	}
	if json.Unmarshal([]byte(s.ProvenanceJSON), &metadata) == nil {
		s.CatalogSourceID = metadata.Catalog.SourceID
		s.CatalogRevision = metadata.Catalog.Revision
		s.AccessRevision = metadata.Catalog.AccessRevision
	}
}
func (s Source) EmbeddingHash() string {
	raw, _ := json.Marshal([]string{s.Title, s.Content})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func (s Source) MemoryOrigin() content.Origin {
	return content.StoredDataOrigin(content.MemoryDerived, "chunk:"+strconv.FormatInt(s.ID, 10), s.ProvenanceJSON)
}

func (s Source) Hash() string {
	raw, _ := json.Marshal([]string{s.Title, s.Content, s.Summary, s.SourceType, s.SourceID, s.ProvenanceJSON})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type Generation struct {
	ID        string
	Space     Space
	Scope     Scope
	State     string
	CreatedAt time.Time
}
type Hit struct {
	Source                Source
	Distance              float64
	GenerationID, SpaceID string
}
type Proof struct {
	ArtifactRef    string  `json:"artifact_ref"`
	ArtifactSHA256 string  `json:"artifact_sha256"`
	CorpusVersion  string  `json:"corpus_version"`
	Cases          int     `json:"cases"`
	BaselineTop1   float64 `json:"baseline_top1"`
	CandidateTop1  float64 `json:"candidate_top1"`
}

func (p Proof) valid() bool {
	_, err := hex.DecodeString(p.ArtifactSHA256)
	return err == nil && len(p.ArtifactSHA256) == 64 && p.ArtifactRef != "" && len(p.ArtifactRef) <= 512 && p.CorpusVersion != "" && p.Cases >= 12 && p.BaselineTop1 >= 0 && p.BaselineTop1 <= 1 && p.CandidateTop1 > p.BaselineTop1 && p.CandidateTop1 <= 1
}
func ValidateVector(v []float32, dims int) error {
	if len(v) != dims || dims != 256 && dims != 768 {
		return ErrInvalid
	}
	var norm float64
	for _, x := range v {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return ErrInvalid
		}
		norm += f * f
	}
	if math.Abs(math.Sqrt(norm)-1) > 0.002 {
		return ErrInvalid
	}
	return nil
}
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
