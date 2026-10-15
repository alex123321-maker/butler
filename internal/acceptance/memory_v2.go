package acceptance

import (
	"encoding/json"
	"math"
	"strings"
)

// V1 remains a retained rejected fixture; V2 criteria are frozen independently.
const MemoryEvalV2 = "butler-memory-eval-v2"
const MemoryInstructionsV2 = MemoryInstructions + ` answer передаёт значение/вывод кратко, без вводных фраз. При утверждении факта из истории обязательны цитаты каждого непосредственно подтверждающего исходника: точная цитата может быть длиннее нужного фрагмента, но должна содержать само подтверждение. Не утверждай факт, если в предоставленном контексте нет достаточных доказательств, даже когда считаешь, что знаешь ответ. Для вопроса о текущем поручении answer и next_action должны согласованно сохранять его ограничения. Вероятность confidence относится к правильности и доказанности утверждения в ЭТОМ контексте; обоснованный abstain использует confidence=0.`

type MemoryCitationRule struct {
	SourceID string `json:"source_id"`
	Revision int64  `json:"revision"`
	Span     string `json:"required_span"`
}

func memoryInstructions(s MemorySuite) string {
	if s.Version == MemoryEvalV2 {
		return MemoryInstructionsV2
	}
	return MemoryInstructions
}

func MemoryOpaqueCaseID(c MemoryCase) string { return memoryCaseID(c) }
func MemoryDigest(value any) string          { return cloudDigest(value) }

// This projection is the complete model-visible payload. Frozen verifier
// captures, rubrics, splits and expected answers never enter it.
func MemoryModelProjection(i MemoryInput) ([]byte, error) {
	return json.Marshal(struct {
		Question string         `json:"question"`
		AsOfUTC  string         `json:"as_of_utc"`
		Task     MemoryTask     `json:"current_task"`
		Memory   []MemorySource `json:"memory_data"`
		Prompt   string         `json:"memory_prompt_data"`
	}{i.Question, i.AsOfUTC, i.Task, i.Memory, i.MemoryPrompt})
}

func MemoryNativeSchema(i MemoryInput) json.RawMessage {
	ids := []string{}
	for _, h := range i.Memory {
		ids = append(ids, h.ID)
	}
	if len(ids) == 0 {
		ids = []string{"__no_source_available__"}
	}
	props := map[string]any{
		"answer": map[string]any{"type": "STRING"}, "abstain": map[string]any{"type": "BOOLEAN"}, "confidence": map[string]any{"type": "NUMBER"},
		"task_revision": map[string]any{"type": "INTEGER"}, "next_action": map[string]any{"type": "STRING", "enum": []string{"none", "wait_for_check", "continue_analysis", "prepare_text", "verify_file", "prepare_invitations", "continue_catalogue", "prepare_ingredients", "check_inventory", "clarify"}},
		"citations": map[string]any{"type": "ARRAY", "minItems": "0", "maxItems": "8", "items": map[string]any{"type": "OBJECT", "required": []string{"source_id", "revision", "quote"}, "properties": map[string]any{"source_id": map[string]any{"type": "STRING", "enum": ids}, "revision": map[string]any{"type": "INTEGER"}, "quote": map[string]any{"type": "STRING"}}}},
	}
	raw, _ := json.Marshal(map[string]any{"type": "OBJECT", "required": []string{"answer", "abstain", "confidence", "citations", "task_revision", "next_action"}, "properties": props})
	return raw
}
func MemoryVersionCriteriaDigest(s MemorySuite) string {
	if s.Version != MemoryEvalV2 {
		return MemoryCriteriaDigest()
	}
	return cloudDigest(struct {
		Version, Instructions, Rule string
		Policy                      MemoryPolicy
		Stages, Arms                []string
	}{MemoryEvalV2, MemoryInstructionsV2, "reviewed-alternatives+anchored-entailing-spans+context-safe-abstention-with-separate-truth-evidence-usefulness+answer-code-consistency-v2", FrozenMemoryV2Policy(), MemoryExpenseStages(), MemoryEvalArms()})
}
func FrozenMemoryV2Policy() MemoryPolicy {
	p := FrozenMemoryPolicy()
	p.MaxCalls = 280
	p.MaxTotalTokens = 4000000
	return p
}
func MemorySuitePolicy(s MemorySuite) MemoryPolicy {
	if s.Version == MemoryEvalV2 {
		return FrozenMemoryV2Policy()
	}
	return FrozenMemoryPolicy()
}
func memoryAnswerValue(v string) string { return strings.ToLower(strings.Join(strings.Fields(v), " ")) }

func memoryAnswerStructureValid(a MemoryAnswer) bool {
	return !math.IsNaN(a.Confidence) && !math.IsInf(a.Confidence, 0) && a.Confidence >= 0 && a.Confidence <= 1 && a.Citations != nil && containsMemory([]string{"none", "wait_for_check", "continue_analysis", "prepare_text", "verify_file", "prepare_invitations", "continue_catalogue", "prepare_ingredients", "check_inventory", "clarify"}, a.NextAction) && a.TaskRevision >= 0 && ((!a.Abstain && strings.TrimSpace(a.Answer) != "") || (a.Abstain && a.Answer == "" && a.Confidence == 0 && len(a.Citations) == 0))
}

// A malformed original is encoded as a JSON string for lossless report storage.
// Unwrap it before hashing/scoring; this is retention, never output repair.
func MemoryOriginalOutput(raw json.RawMessage) []byte {
	var original string
	if json.Unmarshal(raw, &original) == nil {
		return []byte(original)
	}
	return raw
}

func MemoryObservationOriginal(o MemoryObservation) []byte {
	if o.OutputEncoding == "raw_utf8_json_string" {
		return MemoryOriginalOutput(o.Output)
	}
	return o.Output
}

type MemoryAssessment struct {
	SchemaValid         bool `json:"schema_valid"`
	TruthCorrect        bool `json:"truth_correct"`
	EvidenceAvailable   bool `json:"evidence_available"`
	AnswerCorrect       bool `json:"context_answer_correct"`
	CitationCorrect     bool `json:"citation_correct"`
	ApplicableEvidence  bool `json:"applicable_evidence"`
	JustifiedAbstention bool `json:"justified_abstention"`
	FalsePositive       bool `json:"false_positive_assertion"`
	FalseAbstention     bool `json:"false_abstention"`
	TaskCodeCorrect     bool `json:"task_code_correct"`
	InstructionCorrect  bool `json:"answer_instruction_consistent"`
}

// Reviewed alternatives are frozen before inference. Original quoting and
// expected source/revision/span checks do not rely on a second model's verdict.
func AssessMemoryV2(c MemoryCase, i MemoryInput, a MemoryAnswer, validSchema bool) MemoryAssessment {
	validSchema = validSchema && memoryAnswerStructureValid(a)
	x := MemoryAssessment{SchemaValid: validSchema, EvidenceAvailable: len(c.CitationRules) == 0 && !c.Expected.Abstain}
	available := true
	for _, rule := range c.CitationRules {
		found := false
		for _, h := range i.Memory {
			if h.ID == rule.SourceID && h.Revision == rule.Revision && (i.Arm == "retrieval" && strings.Contains(i.MemoryPrompt, rule.Span) || i.Arm != "retrieval" && strings.Contains(h.Body, rule.Span)) {
				found = true
				break
			}
		}
		available = available && found
	}
	if len(c.CitationRules) > 0 {
		x.EvidenceAvailable = available && !c.Expected.Abstain
	}
	if !validSchema {
		return x
	}
	for _, answer := range c.AcceptedAnswers {
		if memoryAnswerValue(a.Answer) == memoryAnswerValue(answer) && !a.Abstain {
			x.TruthCorrect = true
		}
	}
	if c.Expected.Abstain && a.Abstain {
		x.TruthCorrect = true
	}
	x.JustifiedAbstention = a.Abstain && !x.EvidenceAvailable
	x.FalseAbstention = a.Abstain && x.EvidenceAvailable
	x.CitationCorrect = verifyMemoryObservedAnswer(c, i, a) == nil
	if !a.Abstain {
		if len(c.CitationRules) != len(a.Citations) {
			x.CitationCorrect = false
		}
		for _, rule := range c.CitationRules {
			found := false
			for _, w := range a.Citations {
				if w.SourceID == rule.SourceID && w.Revision == rule.Revision && strings.Contains(w.Quote, rule.Span) {
					found = true
				}
			}
			x.CitationCorrect = x.CitationCorrect && found
		}
	}
	x.FalsePositive = !a.Abstain && (!x.TruthCorrect || !x.EvidenceAvailable || !x.CitationCorrect)
	x.AnswerCorrect = x.JustifiedAbstention || !a.Abstain && x.TruthCorrect && x.EvidenceAvailable && x.CitationCorrect
	x.ApplicableEvidence = x.AnswerCorrect && x.CitationCorrect && x.TruthCorrect // Missing an answerable fact is a retrieval miss, even with safe abstention.
	x.TaskCodeCorrect = a.TaskRevision == c.Expected.TaskRevision && a.NextAction == c.Expected.NextAction
	x.InstructionCorrect = x.TaskCodeCorrect && (c.Task.Revision == 0 || x.AnswerCorrect)
	return x
}
