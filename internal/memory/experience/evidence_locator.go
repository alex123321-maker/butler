package experience

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5"
)

type EvidenceRange struct {
	Field      string `json:"field"`
	Offset     int    `json:"offset"`
	MaxChars   int    `json:"max_chars"`
	TotalChars int    `json:"total_chars"`
}
type EvidenceLocator struct {
	Verification           string          `json:"verification"`
	OwnerVerdict           string          `json:"owner_verdict"`
	HistoricalTaskVerified bool            `json:"historical_task_verified"`
	Ranges                 []EvidenceRange `json:"ranges"`
}

// EvidenceLocatorTx annotates only an actual typed, currently authorized lesson.
// The caller must already run full Catalog and lesson applicability proof.
// Offsets address exact source text, not a generated replacement or read receipt.
func (s *Store) EvidenceLocatorTx(ctx context.Context, tx pgx.Tx, src catalog.Source) (json.RawMessage, error) {
	var digest string
	err := tx.QueryRow(ctx, `SELECT lesson_hash FROM memory_experience_lessons WHERE source_id=$1`, src.ID).Scan(&digest)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lesson Lesson
	if skills.Decode([]byte(src.Body), &lesson) != nil || hash(lesson) != digest || lesson.Format != Format || lesson.Observation.Validate() != nil || lesson.Verification != verification(lesson.Observation) {
		return nil, ErrEvidence
	}
	// Open masks credentials before slicing. Never advertise offsets into a
	// different representation, even in an invalid legacy sanitized fixture.
	if sanitize.MaskCredentials(src.Body) != src.Body {
		return nil, nil
	}
	value := EvidenceLocator{Verification: lesson.Verification, OwnerVerdict: lesson.Observation.OwnerVerdict, HistoricalTaskVerified: lesson.Verification == "owner_verified_with_observed_actions"}
	for _, path := range []string{"verification", "observation.owner_verdict", "observation.unknowns", "observation.actions", "observation.failed_calls", "model_notes"} {
		start, end, ok := jsonRange(src.Body, strings.Split(path, "."), 0)
		if !ok {
			return nil, ErrEvidence
		}
		n := utf8.RuneCountInString(src.Body[start:end])
		value.Ranges = append(value.Ranges, EvidenceRange{Field: path, Offset: utf8.RuneCountInString(src.Body[:start]), MaxChars: min(n, 8192), TotalChars: n})
	}
	raw, err := json.Marshal(map[string]any{"experience_evidence_v1": value})
	if err != nil {
		return nil, err
	}
	// Reader locators require the same canonical sanitized representation as
	// Broker/transcript JSON, including nested struct key ordering.
	return json.RawMessage(sanitize.JSON(string(raw))), nil
}

func jsonRange(body string, path []string, base int) (int, int, bool) {
	d := json.NewDecoder(strings.NewReader(body))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return 0, 0, false
	}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return 0, 0, false
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return 0, 0, false
		}
		end := int(d.InputOffset())
		start := end - len(raw)
		if key == path[0] {
			if len(path) == 1 {
				return base + start, base + end, true
			}
			return jsonRange(string(raw), path[1:], base+start)
		}
	}
	return 0, 0, false
}
