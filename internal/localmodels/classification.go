package localmodels

import (
	_ "embed"
	"encoding/json"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

// GLiNERFullScores is an opt-in preprocessing identity. Existing selected-only
// profiles retain their original output and scores for reproducible comparison.
const GLiNERFullScores = "gliner2.5-classification-softmax-v1"
const DecisionSchemaV2 = "butler-event-taxonomy-v2"
const InterpretationSchemaV3 = "butler-event-interpretation-v3"

//go:embed interpretation-schema-v3.json
var interpretationSchema []byte

type InterpretationOption struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type InterpretationDefinition struct {
	Version      string                 `json:"version"`
	Instructions string                 `json:"instructions"`
	Intents      []InterpretationOption `json:"intents"`
	Modality     []InterpretationOption `json:"modality"`
	Target       []InterpretationOption `json:"target"`
}

func InterpretationSchema() ([]byte, InterpretationDefinition) {
	var d InterpretationDefinition
	_ = json.Unmarshal(interpretationSchema, &d)
	return append([]byte(nil), interpretationSchema...), d
}
func InterpretationLabels() []string {
	_, d := InterpretationSchema()
	var labels []string
	for _, group := range []struct {
		prefix  string
		options []InterpretationOption
	}{{"intent/", d.Intents}, {"modality/", d.Modality}, {"target/", d.Target}} {
		for _, o := range group.options {
			labels = append(labels, group.prefix+o.ID)
		}
	}
	return labels
}
func sameInterpretationLabels(labels []string) bool {
	expected := InterpretationLabels()
	if len(labels) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	for _, label := range labels {
		if seen[label] {
			return false
		}
		seen[label] = true
	}
	for _, label := range expected {
		if !seen[label] {
			return false
		}
	}
	return true
}

func fullScoreProfile(p *pb.ModelProfile) bool {
	return (p.PreprocessingVersion == GLiNERFullScores || p.PreprocessingVersion == DecisionSchemaV2 || p.PreprocessingVersion == InterpretationSchemaV3) &&
		(p.Model == "fastino/gliner2.5-multi-v1" || p.Model == "fastino/GLiNER2.5-multi-Decide")
}
