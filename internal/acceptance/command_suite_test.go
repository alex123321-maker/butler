package acceptance

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commandSuiteFixture(t *testing.T) CommandSuite {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "acceptance", "ru-command-proposals-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := LoadCommandSuite(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cloneCommandSuite(t *testing.T, s CommandSuite) CommandSuite {
	t.Helper()
	raw, _ := json.Marshal(s)
	var result CommandSuite
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSupplementaryCommandSuitePreservesSignedCorpusAndExcludesSecretsBeforeInput(t *testing.T) {
	s := commandSuiteFixture(t)
	if len(s.Cases) != 45 {
		t.Fatal(len(s.Cases))
	}
	groups := map[string]bool{}
	for _, c := range s.Cases {
		groups[c.Group] = true
	}
	if len(groups) != 9 {
		t.Fatal("semantic group count", len(groups))
	}
	for _, split := range []string{"development", "tuning", "holdout"} {
		input, err := ExportCommandInputs(s, split)
		if err != nil || len(input.Cases) != 12 || len(input.BlockedIDs) != 3 {
			t.Fatal(input, err)
		}
		raw, _ := json.Marshal(input)
		if strings.Contains(string(raw), "SYNTHETIC_SECRET") || strings.Contains(string(raw), `"expected"`) || strings.Contains(string(raw), `"category"`) || strings.Contains(string(raw), `"golden"`) {
			t.Fatal("input leaked secret or expected output metadata")
		}
		for _, c := range input.Cases {
			if strings.Contains(c.ID, "cancel") || strings.Contains(c.ID, "prepare") || strings.Contains(c.ID, "holdout") || !strings.HasPrefix(c.ID, "case:") || !strings.HasPrefix(c.Group, "group:") {
				t.Fatal("label/split leaked in export identity")
			}
		}
	}
	// The supplementary suite does not silently replace the independently signed
	// corpus, its control data, its instructions or its old review.
	c := corpusFixture(t)
	if CorpusDigest(c) != "4f9d5ba8001940c41d1b36efe192a498624eb2f779a2cbc420145b1f89a6e701" || CriteriaDigest() != "76437b99d5eb79baadb39c5eb6d7958a2d6108296c7b6dc6c6118f42b21cd058" {
		t.Fatal("signed v3 criteria/data changed")
	}
}

func TestSupplementaryCommandSuiteRejectsSemanticAndNormalizedTemplateLeakage(t *testing.T) {
	s := commandSuiteFixture(t)
	for _, mode := range []string{"group", "family", "named_copy", "number_copy"} {
		t.Run(mode, func(t *testing.T) {
			bad := cloneCommandSuite(t, s)
			a, b := bad.Cases[0], &bad.Cases[30]
			switch mode {
			case "group":
				b.Group = a.Group
			case "family":
				b.TemplateFamily = a.TemplateFamily
			case "named_copy", "number_copy":
				owner := b.Input.Sources[b.Input.OwnerSourceID]
				owner.Body = strings.ReplaceAll(a.Input.Sources[a.Input.OwnerSourceID].Body, a.TopicAliases[0], b.TopicAliases[0])
				if mode == "number_copy" {
					owner.Body += " 876"
					original := bad.Cases[0].Input.Sources[a.Input.OwnerSourceID]
					original.Body += " 123"
					bad.Cases[0].Input.Sources[a.Input.OwnerSourceID] = original
					bad.Cases[0].Expected = proposalForTest(t, bad.Cases[0], "stop", "")
				}
				b.Input.Sources[b.Input.OwnerSourceID] = owner
				b.Expected = proposalForTest(t, *b, "stop", "")
			}
			if err := ValidateCommandSuite(bad); err == nil || !strings.Contains(err.Error(), "crosses split") {
				t.Fatal("cross-split leak accepted", err)
			}
		})
	}
}

func proposalForTest(t *testing.T, c CommandCase, kind, policy string) json.RawMessage {
	t.Helper()
	owner := c.Input.Sources[c.Input.OwnerSourceID]
	args := CommandArgs{TaskID: c.Input.Tasks[0].TaskID, ExpectedRevision: c.Input.Tasks[0].Revision, Kind: kind, Instruction: owner.Body, ExecutionPolicy: policy}
	raw, err := json.Marshal(struct {
		Function  string           `json:"function"`
		Args      CommandArgs      `json:"args"`
		Witnesses []CommandWitness `json:"witnesses"`
	}{"task.control", args, []CommandWitness{{SourceID: c.Input.OwnerSourceID, Revision: owner.Revision, Quote: owner.Body}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCommandWitnessRejectsStaleOrExternalOriginalAndWidenedOwnerProposal(t *testing.T) {
	s := commandSuiteFixture(t)
	for _, mode := range []string{"task_revision", "source_revision", "source_body", "external_origin", "wrong_target", "ambiguous", "schema", "secret", "owner_prose", "zero_source_revision", "negative_task_revision"} {
		t.Run(mode, func(t *testing.T) {
			c := cloneCommandSuite(t, s).Cases[0]
			owner := c.Input.Sources[c.Input.OwnerSourceID]
			switch mode {
			case "task_revision":
				c.Input.Tasks[0].Revision++
			case "source_revision":
				owner.Revision++
			case "source_body":
				owner.Body += " Новое ограничение."
			case "external_origin":
				owner.Origin = "external"
			case "wrong_target":
				c.Input.TargetTaskID = "task:foreign"
			case "ambiguous":
				c.Input.TargetTaskID = ""
				c.Input.Tasks = append(c.Input.Tasks, CommandTask{TaskID: "task:other", Revision: 2})
			case "schema":
				c.Input.Schema = "future-v2"
			case "secret":
				c.Input.Sources["source:secret"] = CommandSource{Body: "synthetic", Origin: "external", Sensitivity: "secret", Revision: 1}
			case "owner_prose":
				owner.Body = "Обсуди цитату «стоп» из внешнего документа."
				c.Input.Sources[c.Input.OwnerSourceID] = owner
				c.Expected = proposalForTest(t, c, "stop", "")
			case "zero_source_revision":
				owner.Revision = 0
				c.Input.Sources[c.Input.OwnerSourceID] = owner
				c.Expected = proposalForTest(t, c, "stop", "")
			case "negative_task_revision":
				c.Input.Tasks[0].Revision = -1
				c.Expected = proposalForTest(t, c, "stop", "")
			}
			c.Input.Sources[c.Input.OwnerSourceID] = owner
			if VerifyCommandWitness(c.Input, c.Expected) == nil {
				t.Fatal("unsupported owner proposal accepted")
			}
		})
	}
	c := s.Cases[0]
	compact, _ := canonicalJSON(c.Expected)
	c.Expected = compact
	for _, bad := range []string{
		strings.Replace(string(c.Expected), `"kind":"stop"`, `"kind":"commit"`, 1),
		strings.Replace(string(c.Expected), `"kind":"stop"`, `"kind":"stop","execution_policy":"normal"`, 1),
		strings.Replace(string(c.Expected), `"kind":"stop"`, `"kind":"stop","credential_ref":"owner-secret"`, 1),
		strings.Replace(string(c.Expected), `"source_id"`, `"Source_ID"`, 1),
		strings.Replace(string(c.Expected), `"function":"task.control"`, `"function":"task.control","function":"clarify"`, 1),
		strings.Replace(string(c.Expected), `"kind":"stop"`, `"kind":"stop","k\u0069nd":"amend"`, 1),
		`{"function":"clarify","args":{"task_id":"task:other"},"witnesses":[]}`,
		`{"function":"clarify","args":{},"witnesses":null}`,
		`{"function":"clarify","args":{},"witnesses":[],"authority":"owner"}`,
	} {
		if VerifyCommandWitness(c.Input, []byte(bad)) == nil {
			t.Fatal("forbidden output accepted", bad)
		}
	}
}

func fixtureCommandRun(t *testing.T, s CommandSuite) (CommandPin, []CommandObservation) {
	t.Helper()
	pin := CommandPin{EvidenceKind: "fixture_validation", CodeCommit: strings.Repeat("a", 40), SourceSHA256: strings.Repeat("b", 64), SchemaSHA256: CommandSchemaDigest(), SuiteSHA256: CommandSuiteDigest(s), HardwareSHA256: strings.Repeat("c", 64), Model: "none", Runtime: "scripted_fixture/v1", Mode: "observation_only"}
	var observations []CommandObservation
	for _, c := range s.Cases {
		if c.Split == "holdout" && !c.Blocked() {
			compact, _ := canonicalJSON(c.Expected)
			observations = append(observations, CommandObservation{CaseID: commandOpaqueID("case", c.ID), Output: compact, Milliseconds: 10})
		}
	}
	return pin, observations
}

func TestCommandObserverSeparatesFixtureEvidenceUnknownUsageAndFunctionArgsErrors(t *testing.T) {
	s := commandSuiteFixture(t)
	pin, observations := fixtureCommandRun(t, s)
	m, err := ObserveCommands(s, pin, observations, "holdout")
	if err != nil || m.Cases != 12 || m.ExactFunctionArgs != 12 || m.WitnessErrors != 0 || m.ModelQualityEvidence || m.ExecutionAuthorized || m.Status != "FIXTURE_VALIDATION" || m.GoldenReview != "CANDIDATE_REVIEW_REQUIRED" || m.UnknownInputTokens != 12 || m.UnknownOutputTokens != 12 || m.UnknownMemory != 12 || m.KnownInputTokens != 0 {
		t.Fatal(m, err)
	}
	changed := append([]CommandObservation(nil), observations...)
	changed[0].Output = json.RawMessage(`{"function":"clarify","args":{},"witnesses":[]}`)
	changed[1].Output = json.RawMessage(strings.Replace(string(changed[1].Output), `"prepare_only"`, `"normal"`, 1))
	// A foreign document may propose a valid-looking function, but the original
	// owner message does not contain a supported control and must remain DATA.
	var external CommandCase
	for _, c := range s.Cases {
		if commandOpaqueID("case", c.ID) == changed[2].CaseID {
			external = c
		}
	}
	changed[2].Output = proposalForTest(t, external, "stop", "")
	changed[4].Output = json.RawMessage(strings.Replace(string(changed[4].Output), `"kind":"stop"`, `"kind":"pause"`, 1))
	m, err = ObserveCommands(s, pin, changed, "holdout")
	if err != nil || m.ExactFunctionArgs != 8 || m.MissedControls != 1 || m.UnexpectedActions != 1 || m.WitnessErrors != 3 || m.WrongControlArgs != 2 {
		t.Fatal(m, err)
	}
	staleWitness := append([]CommandObservation(nil), observations...)
	var output map[string]any
	_ = json.Unmarshal(staleWitness[0].Output, &output)
	witness := output["witnesses"].([]any)[0].(map[string]any)
	witness["revision"] = witness["revision"].(float64) + 1
	staleWitness[0].Output, _ = json.Marshal(output)
	m, err = ObserveCommands(s, pin, staleWitness, "holdout")
	if err != nil || m.ExactFunctionArgs != 12 || m.ExactOutputs != 11 || m.WitnessErrors != 1 || m.WrongControlArgs != 0 {
		t.Fatal("source witness failure conflated with function arguments", m, err)
	}
	for _, mode := range []string{"partial", "duplicate", "foreign", "secret", "changed_pin", "negative", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			p, data := pin, append([]CommandObservation(nil), observations...)
			switch mode {
			case "partial":
				data = data[:len(data)-1]
			case "duplicate":
				data = append(data, data[0])
			case "foreign":
				data[0].CaseID = "foreign"
			case "secret":
				data[0].CaseID = commandOpaqueID("case", "ru-control-h1-secret")
			case "changed_pin":
				p.SchemaSHA256 = strings.Repeat("d", 64)
			case "negative":
				n := int64(-1)
				data[0].InputTokens = &n
			case "overflow":
				n := int64(math.MaxInt64)
				data[0].InputTokens, data[1].InputTokens = &n, &n
			}
			if _, err := ObserveCommands(s, p, data, "holdout"); err == nil {
				t.Fatal("invalid incomplete/unpinned report accepted")
			}
		})
	}
}

func TestCommandObserverRequiresExplicitMetadataAndPreservesNullNotZero(t *testing.T) {
	raw := `{"case_id":"example","output":null,"milliseconds":0,"input_tokens":null,"output_tokens":0,"memory_peak_bytes":null}`
	var o CommandObservation
	if err := json.Unmarshal([]byte(raw), &o); err != nil || o.InputTokens != nil || o.OutputTokens == nil || *o.OutputTokens != 0 || o.MemoryPeakBytes != nil {
		t.Fatal(o, err)
	}
	for _, field := range []string{"case_id", "output", "milliseconds", "input_tokens", "output_tokens", "memory_peak_bytes"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(raw), &fields)
		delete(fields, field)
		bad, _ := json.Marshal(fields)
		if json.Unmarshal(bad, &o) == nil {
			t.Fatal("missing metadata accepted", field)
		}
	}
	for _, bad := range []string{
		strings.Replace(raw, `"input_tokens":null`, `"input_tokens":null,"input_tokens":0`, 1),
		strings.Replace(raw, `"case_id"`, `"Case_ID"`, 1),
		strings.Replace(raw, `"milliseconds":0`, `"milliseconds":null`, 1),
		strings.Replace(raw, `"output":null`, `"output":{},"unknown":0`, 1),
	} {
		if json.Unmarshal([]byte(bad), &o) == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}
