package delegation

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const NumericObservationKind = "delegation_numeric_observation_v1"

// NumericObservation is source-relative DATA, never an accomplishment or an
// authorization. Its original source must be read under current Catalog proof.
type NumericObservation struct {
	SourceID       string `json:"source_id"`
	SourceRevision int64  `json:"source_revision"`
	SourceHash     string `json:"source_hash"`
	ObservedValue  int64  `json:"observed_value"`
	ObservedDate   string `json:"observed_date"`
	Uncertainty    string `json:"uncertainty"`
}

// NumericResearchSource recognizes an explicit versioned Catalog contract body.
// The marker belongs in TEXT, since JSONB locators discard duplicate keys.
// The selected source is an additional acceptance constraint, never authority.
func NumericResearchSource(body []byte) (SourceRef, bool, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return SourceRef{}, false, nil
	}
	if len(body) > 8192 || !json.Valid(body) {
		return SourceRef{}, false, ErrDenied
	}
	// Scan every top-level occurrence before strict decoding. A duplicate final
	// legacy marker cannot hide an earlier opt-in by last-value-wins decoding.
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, _ := decoder.Token()
	if start != json.Delim('{') {
		return SourceRef{}, false, nil
	}
	applicable := false
	for decoder.More() {
		key, _ := decoder.Token()
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return SourceRef{}, false, ErrDenied
		}
		var kind string
		if name, ok := key.(string); ok && strings.EqualFold(name, "kind") && json.Unmarshal(raw, &kind) == nil && strings.HasPrefix(kind, "delegation_numeric_observation") {
			applicable = true
		}
	}
	if !applicable {
		return SourceRef{}, false, nil
	}
	var contract struct {
		Kind   string     `json:"kind"`
		Source *SourceRef `json:"source_ref"`
	}
	if strictJSON(body, &contract) != nil || contract.Kind != NumericObservationKind || contract.Source == nil || !ValidSource(*contract.Source) {
		return SourceRef{}, true, ErrDenied
	}
	return *contract.Source, true, nil
}

func ParseNumericObservation(raw []byte) (NumericObservation, error) {
	var wire struct {
		SourceID       *string `json:"source_id"`
		SourceRevision *int64  `json:"source_revision"`
		SourceHash     *string `json:"source_hash"`
		ObservedValue  *int64  `json:"observed_value"`
		ObservedDate   *string `json:"observed_date"`
		Uncertainty    *string `json:"uncertainty"`
	}
	if strictJSON(raw, &wire) != nil || wire.SourceID == nil || wire.SourceRevision == nil || wire.SourceHash == nil || wire.ObservedValue == nil || wire.ObservedDate == nil || wire.Uncertainty == nil || !validID(*wire.SourceID) || *wire.SourceRevision < 1 || !validHash(*wire.SourceHash) || !validObservationDate(*wire.ObservedDate) || *wire.Uncertainty != "source_only" {
		return NumericObservation{}, ErrDenied
	}
	return NumericObservation{*wire.SourceID, *wire.SourceRevision, *wire.SourceHash, *wire.ObservedValue, *wire.ObservedDate, *wire.Uncertainty}, nil
}

// VerifyNumericObservation compares one final against an exact selected witness.
// The caller must independently load both immutable final and current original
// Catalog body, checking source identity/hash/access and their owning task/run.
// Agreement with another final is deliberately irrelevant to this check.
func VerifyNumericObservation(raw []byte, source SourceRef, witness []byte) error {
	if !ValidSource(source) {
		return ErrDenied
	}
	observation, err := ParseNumericObservation(raw)
	if err != nil || observation.SourceID != source.ID || observation.SourceRevision != source.Revision || observation.SourceHash != source.Hash {
		return ErrDenied
	}
	var fact struct {
		Value *int64  `json:"observed_value"`
		Date  *string `json:"observed_date"`
	}
	if strictJSON(witness, &fact) != nil || fact.Value == nil || fact.Date == nil || !validObservationDate(*fact.Date) || observation.ObservedValue != *fact.Value || observation.ObservedDate != *fact.Date {
		return ErrDenied
	}
	return nil
}

func validObservationDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func strictJSON(raw []byte, value any) error {
	if len(raw) == 0 || len(raw) > 8192 {
		return ErrDenied
	}
	keys := json.NewDecoder(bytes.NewReader(raw))
	if uniqueReportValue(keys, 0) != nil {
		return ErrDenied
	}
	if _, err := keys.Token(); err != io.EOF {
		return ErrDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrDenied
	}
	return nil
}
