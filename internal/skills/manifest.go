// Package skills owns immutable procedural DATA. Skills cannot authorize tools,
// alter owner policy, execute scripts, or contain mutable filesystem resources.
package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
)

const MaxBytes = 40 * 1024 // private envelope / tool argument bound
const MaxManifestBytes = 16 * 1024
const Format = "butler.skill.v1"

var (
	ErrManifest    = errors.New("skill manifest rejected")
	ErrAccess      = errors.New("current skill owner or task authority required")
	ErrConflict    = errors.New("skill version or dependencies changed")
	ErrEvidence    = errors.New("independent verified task evidence required")
	ErrUnavailable = errors.New("reviewed skill unavailable")
	identifier     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	version        = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9.-]+)?$`)
	digest         = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Source struct {
	URI      string `json:"uri"`
	Revision string `json:"revision"`
}
type EvidenceRef struct {
	TaskID       string `json:"task_id"`
	TaskRevision int64  `json:"task_revision"`
	ResultID     string `json:"result_id"`
}
type Dependency struct {
	Tool             string `json:"tool"`
	ContractHash     string `json:"contract_hash"`
	ContractRevision string `json:"contract_revision"`
}
type Step struct {
	ID                 string `json:"id"`
	Tool               string `json:"tool"`
	Intent             string `json:"intent"`
	SuccessCheck       string `json:"success_check"`
	OnError            string `json:"on_error"`
	Retry              string `json:"retry"`
	ApprovalCheckpoint bool   `json:"approval_checkpoint"`
}
type Scenario struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Expected string `json:"expected"`
}
type Manifest struct {
	Format        string              `json:"format"`
	ID            string              `json:"id"`
	Version       string              `json:"version"`
	Source        Source              `json:"source"`
	Description   string              `json:"description"`
	When          []string            `json:"when"`
	Inputs        []string            `json:"inputs"`
	NotApplicable []string            `json:"not_applicable"`
	Dependencies  []Dependency        `json:"dependencies"`
	Steps         []Step              `json:"steps"`
	Tests         []Scenario          `json:"tests"`
	Sources       []EvidenceRef       `json:"sources"`
	LessonSources []catalog.SourceRef `json:"lesson_sources,omitempty"`
}
type Package struct {
	Manifest Manifest `json:"manifest"`
	Digest   string   `json:"digest"`
	Raw      []byte   `json:"-"`
}

func Decode(raw []byte, target any) error {
	if len(raw) > MaxBytes {
		return ErrManifest
	}
	// ArgumentsHash rejects duplicates and depth ambiguity before decoding.
	if _, err := actionauth.ArgumentsHash(string(raw)); err != nil {
		return ErrManifest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return ErrManifest
	}
	return nil
}
func Parse(raw []byte) (Package, error) {
	var m Manifest
	if len(raw) > MaxManifestBytes || Decode(raw, &m) != nil || validate(m) != nil {
		return Package{}, ErrManifest
	}
	sum := sha256.Sum256(raw)
	return Package{m, hex.EncodeToString(sum[:]), append([]byte(nil), raw...)}, nil
}
func validStrings(values []string, max int) bool {
	if len(values) == 0 || len(values) > max {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if strings.TrimSpace(v) == "" || len(v) > 2048 || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func validate(m Manifest) error {
	if len(m.LessonSources) > 8 {
		return ErrManifest
	}
	lessonIDs := map[string]bool{}
	for _, r := range m.LessonSources {
		if r.ID == "" || len(r.ID) > 512 || r.Revision < 1 || r.AccessRevision < 1 || !digest.MatchString(r.Hash) || lessonIDs[r.ID] {
			return ErrManifest
		}
		lessonIDs[r.ID] = true
	}
	u, err := url.Parse(m.Source.URI)
	if m.Format != Format || !identifier.MatchString(m.ID) || !version.MatchString(m.Version) || err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(m.Source.Revision) == "" || len(m.Source.Revision) > 160 || strings.TrimSpace(m.Description) == "" || len(m.Description) > 512 || !validStrings(m.When, 8) || !validStrings(m.Inputs, 16) || !validStrings(m.NotApplicable, 16) || len(m.Dependencies) == 0 || len(m.Dependencies) > 16 || len(m.Steps) == 0 || len(m.Steps) > 24 || len(m.Sources) == 0 || len(m.Sources) > 8 || len(m.Tests) != 3 {
		return ErrManifest
	}
	tools := map[string]bool{}
	for _, d := range m.Dependencies {
		if d.Tool == "" || len(d.Tool) > 160 || strings.ContainsAny(d.Tool, "*? /\\\x00") || !strings.Contains(d.Tool, ".") || !digest.MatchString(d.ContractHash) || d.ContractRevision == "" || len(d.ContractRevision) > 160 || tools[d.Tool] {
			return ErrManifest
		}
		tools[d.Tool] = true
	}
	steps := map[string]bool{}
	for _, s := range m.Steps {
		if !identifier.MatchString(s.ID) || steps[s.ID] || !tools[s.Tool] || strings.TrimSpace(s.Intent) == "" || len(s.Intent) > 2048 || strings.TrimSpace(s.SuccessCheck) == "" || len(s.SuccessCheck) > 2048 || s.OnError != "stop" || (s.Retry != "read_again" && s.Retry != "reconcile" && s.Retry != "never") {
			return ErrManifest
		}
		steps[s.ID] = true
	}
	kinds := map[string]bool{}
	for _, t := range m.Tests {
		if !identifier.MatchString(t.Name) || strings.TrimSpace(t.Expected) == "" || len(t.Expected) > 1024 || kinds[t.Kind] || (t.Kind != "verified_success" && t.Kind != "failure_stop" && t.Kind != "interface_change") {
			return ErrManifest
		}
		kinds[t.Kind] = true
	}
	seen := map[string]bool{}
	for _, r := range m.Sources {
		if r.TaskID == "" || len(r.TaskID) > 160 || r.TaskRevision <= 0 || r.ResultID == "" || len(r.ResultID) > 160 || seen[r.ResultID] {
			return ErrManifest
		}
		seen[r.ResultID] = true
	}
	return nil
}

// Narrow correction grants permit only descriptive wording changes. The exact
// dependencies, steps, checks, inputs, retry policy and sources remain pinned.
func CorrectionShape(m Manifest) string {
	m.Version = ""
	m.Source.Revision = ""
	m.Description = ""
	m.Steps = append([]Step(nil), m.Steps...)
	for i := range m.Steps {
		m.Steps[i].Intent = ""
	}
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
