// Command memory-adaptive-capture diagnoses retained logical native captures.
// It preserves exact raw and expected hashes; the typed-origin hypothesis is
// diagnostic only and never rewrites originals or grants qualification.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type sessionRef struct {
	ProviderName, SessionRef, ResponseRef string
	CreatedAt, LastUsedAt                 time.Time
}
type runContext struct {
	RunID, SessionKey, ProviderName, ModelName                   string
	ProviderSessionRef                                           *sessionRef
	SupportsStreaming, SupportsToolCalls, SupportsStatefulResume bool
}
type inputItem struct {
	Role, Content, ContentType, Name string
	Metadata                         map[string]any
}
type toolDefinition struct{ Name, Description, SchemaJSON string }
type startRequest struct {
	Context              runContext
	InputItems           []inputItem
	ToolDefinitions      []toolDefinition
	StreamingEnabled     bool
	TransportOptions     map[string]any
	TransportOptionsJSON string
}
type continuationRequest struct {
	RunID                       string
	ProviderSessionRef          *sessionRef
	ToolCallRef, ToolResultJSON string
	TransportOptions            map[string]any
	TransportOptionsJSON        string
}
type usage struct{ InputTokens, OutputTokens, TotalTokens int }

// Exact frozen content.Origin shape; values are never normalized or repaired.
type origin struct {
	Kind        string   `json:"kind"`
	SourceRefs  []string `json:"source_refs"`
	ParentRefs  []string `json:"parent_refs,omitempty"`
	Sensitivity string   `json:"sensitivity"`
}
type typedCall struct {
	ID             int64                `json:"id"`
	OperationID    string               `json:"operation_id"`
	ReservationID  string               `json:"reservation_id"`
	RunID          string               `json:"run_id"`
	Kind           string               `json:"kind"`
	Start          *startRequest        `json:"start,omitempty"`
	Continuation   *continuationRequest `json:"continuation,omitempty"`
	Usage          *usage               `json:"usage"`
	InvalidUsage   bool                 `json:"invalid_usage"`
	FullTotalKnown bool                 `json:"full_total_known"`
	Final          string               `json:"final"`
	ModelVersion   string               `json:"model_version"`
	Failure        string               `json:"failure"`
	Millis         int64                `json:"millis"`
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

type captureDigest struct {
	RunID                  string `json:"run_id"`
	Entries                int    `json:"entries"`
	SHA256                 string `json:"retained_raw_sha256"`
	TypedSHA256            string `json:"typed_roundtrip_sha256"`
	Expected               string `json:"expected_capture_sha256"`
	OriginHypothesisSHA256 string `json:"provenance_struct_hypothesis_sha256"`
	RetainedMatches        bool   `json:"retained_matches_expected"`
	TypedMatches           bool   `json:"typed_matches_retained"`
	HypothesisMatches      bool   `json:"hypothesis_matches_expected"`
	ProvenanceItems        int    `json:"provenance_items"`
	SessionReferences      int    `json:"session_references"`
	NonzeroTimestamps      int    `json:"nonzero_timestamps"`
}

func run(proof string) error {
	raw, err := os.ReadFile(filepath.Join(proof, "native-calls.json"))
	if err != nil {
		return err
	}
	var calls []json.RawMessage
	if err = json.Unmarshal(raw, &calls); err != nil {
		return err
	}
	groups := map[string][]json.RawMessage{}
	for _, call := range calls {
		var id struct {
			RunID string `json:"run_id"`
		}
		if json.Unmarshal(call, &id) != nil || id.RunID == "" {
			return fmt.Errorf("native identity unavailable")
		}
		groups[id.RunID] = append(groups[id.RunID], call)
	}
	packet, err := os.Open(filepath.Join(proof, "review-cases.jsonl"))
	if err != nil {
		return err
	}
	defer packet.Close()
	expected := map[string]string{}
	scanner := bufio.NewScanner(packet)
	scanner.Buffer(make([]byte, 4096), 32<<20)
	for scanner.Scan() {
		var row struct {
			RunID       string `json:"run_id"`
			Correlation struct {
				Expected string `json:"expected_capture_sha256"`
			} `json:"capture_correlation"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.RunID == "" || len(row.Correlation.Expected) != 64 || expected[row.RunID] != "" {
			return fmt.Errorf("original capture identity unavailable")
		}
		expected[row.RunID] = row.Correlation.Expected
	}
	if scanner.Err() != nil || len(expected) != len(groups) {
		return fmt.Errorf("original capture population differs")
	}
	var result []captureDigest
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		compact, e := json.Marshal(groups[key])
		if e != nil {
			return e
		}
		var typed []typedCall
		if json.Unmarshal(compact, &typed) != nil {
			return fmt.Errorf("typed native capture unavailable")
		}
		typedBytes, e := json.Marshal(typed)
		if e != nil {
			return e
		}
		row := captureDigest{RunID: key, Entries: len(groups[key]), SHA256: digest(compact), TypedSHA256: digest(typedBytes), Expected: expected[key]}
		for i := range typed {
			var refs []*sessionRef
			if typed[i].Start != nil {
				refs = append(refs, typed[i].Start.Context.ProviderSessionRef)
				for j := range typed[i].Start.InputItems {
					item := &typed[i].Start.InputItems[j]
					if value, ok := item.Metadata["butler_provenance"]; ok {
						encoded, e := json.Marshal(value)
						if e != nil {
							return e
						}
						var restored origin
						if json.Unmarshal(encoded, &restored) != nil {
							return fmt.Errorf("retained provenance unavailable")
						}
						item.Metadata["butler_provenance"] = restored
						row.ProvenanceItems++
					}
				}
			}
			if typed[i].Continuation != nil {
				refs = append(refs, typed[i].Continuation.ProviderSessionRef)
			}
			for _, ref := range refs {
				if ref == nil {
					continue
				}
				row.SessionReferences++
				if !ref.CreatedAt.IsZero() {
					row.NonzeroTimestamps++
				}
				if !ref.LastUsedAt.IsZero() {
					row.NonzeroTimestamps++
				}
			}
		}
		hypothesis, e := json.Marshal(typed)
		if e != nil {
			return e
		}
		row.OriginHypothesisSHA256 = digest(hypothesis)
		row.RetainedMatches = row.SHA256 == row.Expected
		row.TypedMatches = row.TypedSHA256 == row.SHA256
		row.HypothesisMatches = row.OriginHypothesisSHA256 == row.Expected
		result = append(result, row)
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(proof, "native-capture-diagnosis.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(output, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
func main() {
	if len(os.Args) != 2 || run(os.Args[1]) != nil {
		fmt.Fprintln(os.Stderr, "retained native capture diagnosis unavailable")
		os.Exit(1)
	}
}
