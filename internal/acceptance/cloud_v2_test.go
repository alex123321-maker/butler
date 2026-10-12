package acceptance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Mechanical fixture only. These random-token templates are deliberately not
// the authored Russian corpus, an independent holdout or model-quality evidence.
func cloudV2Fixture(t *testing.T) (CloudV2Corpus, GoldenApproval, []byte, CloudPin, []CloudAttempt) {
	t.Helper()
	c := CloudV2Corpus{Version: CloudV2CorpusVersion, ComparisonCaseIDs: []string{"q201a", "q202c", "q203e", "q204g"}}
	for i := 0; i < 30; i++ {
		group := fmt.Sprintf("g%d", 201+i)
		split := "development"
		if i >= 12 {
			split = "tuning"
		}
		if i >= 20 {
			split = "holdout"
		}
		origin := "synthetic_public"
		if i == 29 {
			origin = "synthetic_secret"
		}
		c.Groups = append(c.Groups, CloudV2Group{group, split, "semantic-" + group, "mechanism-" + group, origin})
		for j, kind := range []string{"retrieval", "retrieval", "extraction", "extraction", "classification", "classification", "commands", "commands"} {
			id := fmt.Sprintf("q%d%c", 201+i, 'a'+j)
			doc := fmt.Sprintf("src%dx", 201+i)
			value := "VALUE-" + cloudDigest(group)[:24]
			e := CloudV2Case{ID: id, Group: group, Kind: kind, Input: kind + " " + cloudDigest(id), Documents: map[string]string{}}
			var golden any
			switch kind {
			case "retrieval":
				e.Documents[doc] = "record " + cloudDigest(group) + " exact " + value
				refs := []string{}
				if j == 0 {
					refs = append(refs, doc)
				}
				golden = map[string]any{"refs": refs}
			case "extraction":
				e.Documents[doc] = "record " + cloudDigest(group) + " exact " + value
				facts := []any{}
				if j == 2 {
					facts = append(facts, map[string]string{"key": "field", "value": value, "source_id": doc, "quote": value})
				}
				golden = map[string]any{"facts": facts}
			case "classification":
				label := "important"
				if j == 5 {
					label = "observe"
				}
				golden = map[string]string{"label": label}
			case "commands":
				intent, authority := "stop_tree", "owner_required"
				if j == 7 {
					intent, authority = "clarify", "none"
				}
				golden = map[string]string{"intent": intent, "authority": authority}
			}
			e.Golden, _ = json.Marshal(golden)
			e.NoModel = i == 29 && len(e.Documents) > 0
			c.Cases = append(c.Cases, e)
		}
	}
	if err := ValidateCloudV2Corpus(c); err != nil {
		t.Fatal("mechanical fixture invalid", err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := GoldenApproval{Digest: CloudV2CorpusDigest(c), CriteriaDigest: CloudV2CriteriaDigest(c), Rules: CloudV2Protocol, Reviewer: "independent-unit-fixture", CandidateAuthor: "unit-fixture-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(private, unsigned))
	p := CloudPin{EvidenceKind: "synthetic_fixture", Provider: CloudProvider, RequestedModel: CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "unit-only", RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: strings.Repeat("c", 64), HardwareSHA256: strings.Repeat("d", 64), CorpusDigest: CloudV2CorpusDigest(c), SchemaSHA256: CloudV2SchemaDigest(), PolicySHA256: CloudV2PolicyDigest(c), ResourcesSHA256: CloudV2ResourcesDigest(), WindowID: "unit-fixture-never-native", Mode: "shadow_observation"}
	inputs, err := ExportCloudV2Inputs(c, "")
	if err != nil {
		t.Fatal(err)
	}
	gold := map[string]json.RawMessage{}
	for _, e := range c.Cases {
		gold[cloudV2CaseID(e.ID)] = e.Golden
	}
	attempts := []CloudAttempt{}
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for i, input := range inputs.Cases {
		model, _ := MakeCloudV2ModelInput(input, CloudRoleSingle, nil)
		start := at.Add(time.Duration(i) * time.Second)
		attempts = append(attempts, CloudAttempt{AttemptID: fmt.Sprintf("fixture-%d", i), CaseID: input.ID, ContextID: fmt.Sprintf("context-%d", i), Role: CloudRoleSingle, ProviderModelVersion: stringCloud("gemini-3.8-flash-n"), StartUTC: start, EndUTC: start.Add(10 * time.Millisecond), Milliseconds: 10, InputSHA256: CloudV2ModelInputDigest(model), Output: gold[input.ID], Outcome: "completed", InputTokens: intCloud(10), OutputTokens: intCloud(8), TotalTokens: intCloud(20), LocalPeakRSSBytes: intCloud(1024)})
	}
	return c, a, public, p, attempts
}
func TestCloudV2ExplicitCorpusAndOpaqueInputContract(t *testing.T) {
	c, _, _, _, _ := cloudV2Fixture(t)
	raw, _ := json.Marshal(c)
	loaded, err := LoadCloudV2Corpus(bytes.NewReader(raw))
	if err != nil || CloudV2CorpusDigest(loaded) != CloudV2CorpusDigest(c) {
		t.Fatal("explicit reader", err)
	}
	x, err := ExportCloudV2Inputs(c, "")
	if err != nil || len(x.Cases) != 236 || len(x.BlockedIDs) != 4 || x.Protocol != CloudV2Protocol {
		t.Fatal("secret gate", err)
	}
	encoded, _ := json.Marshal(x)
	for _, forbidden := range []string{`"golden"`, `"group"`, `"attack_family"`, `"semantic_family"`, cloudV2CaseID("q230a") + `","kind"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatal("metadata/blocked case leaked")
		}
	}
	input := x.Cases[0]
	model, err := MakeCloudV2ModelInput(input, CloudRoleSingle, nil)
	if err != nil || model.ResponseMIMEType != "application/json" || model.MaximumGeneratedTokens != 32768 || len(model.Schema) > 16384 || bytes.Contains(model.Schema, []byte("$ref")) || bytes.Contains(model.Schema, []byte(input.ID)) {
		t.Fatal("kind-only native contract", err)
	}
	changed := model
	changed.MaximumGeneratedTokens = 8192
	if CloudV2ModelInputDigest(changed) == CloudV2ModelInputDigest(model) {
		t.Fatal("generation allowance not pinned")
	}
	for _, kind := range []string{"retrieval", "extraction", "classification", "commands"} {
		schema, e := CloudV2ResponseSchema(kind)
		if e != nil || !json.Valid(schema) || !bytes.Contains(schema, []byte(`"additionalProperties":false`)) {
			t.Fatal("strict native schema", kind)
		}
	}
	if CloudV2SchemaDigest() == CloudSchemaDigest() || CloudV2PolicyDigest(c) == CloudPolicyDigest() || CloudPolicyDigest() != "20313d5ec68ef865f51fa0c49a10f8ae9443a66f1e4e2a51c309a1d67073fafb" || CloudSchemaDigest() != "a0009cc050b4f46f5efdc84efec67bcbc07dd3d18add61c34e28ccb330e422f2" {
		t.Fatal("V1 boundary changed")
	}
}
func TestCloudV2ReviewAndStrictCorpusDenials(t *testing.T) {
	c, a, key, _, _ := cloudV2Fixture(t)
	if VerifyCloudV2Golden(c, a, key) != nil {
		t.Fatal("fixture approval")
	}
	if _, _, err := DecodeCloudV2GoldenArtifacts(c, nil, nil); err == nil {
		t.Fatal("unreviewed V2 constants acquired approval")
	}
	keyFile := []byte(hex.EncodeToString(key) + "\n")
	approvalFile, _ := json.Marshal(a)
	if _, _, err := DecodeCloudV2GoldenArtifactsWithPins(c, keyFile, approvalFile, hashWindowBytes(keyFile), hashWindowBytes(approvalFile)); err != nil {
		t.Fatal("independent fixture pins", err)
	}
	if _, _, err := DecodeCloudV2GoldenArtifactsWithPins(c, keyFile, approvalFile, CloudGoldenPublicKeyFileSHA256, CloudGoldenApprovalFileSHA256); err == nil {
		t.Fatal("old V1 review reused")
	}
	raw, _ := json.Marshal(c)
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"version"`), []byte(`"Version"`), 1), bytes.Replace(raw, []byte(`"no_model":false`), []byte(`"no_model":false,"no_model":true`), 1), append(raw, []byte(`{}`)...), bytes.Repeat([]byte("x"), (2<<20)+1)} {
		if _, err := LoadCloudV2Corpus(bytes.NewReader(bad)); err == nil {
			t.Fatal("strict corpus accepted alias/duplicate/trailing/oversize")
		}
	}
	for _, name := range []string{"family_cross_split", "wrong_secret", "comparison_holdout", "nonanchored_gold", "version"} {
		t.Run(name, func(t *testing.T) {
			var bad CloudV2Corpus
			_ = json.Unmarshal(raw, &bad)
			switch name {
			case "family_cross_split":
				bad.Groups[20].SemanticFamily = bad.Groups[0].SemanticFamily
			case "wrong_secret":
				bad.Cases[0].NoModel = true
			case "comparison_holdout":
				bad.ComparisonCaseIDs[0] = "q221a"
			case "nonanchored_gold":
				bad.Cases[2].Golden = json.RawMessage(`{"facts":[{"key":"field","value":"fabricated","source_id":"src201x","quote":"fabricated"}]}`)
			case "version":
				bad.Version = "butler-ru-240-v3"
			}
			if ValidateCloudV2Corpus(bad) == nil {
				t.Fatal("invalid prospective inventory accepted")
			}
		})
	}
}
func TestCloudV2ScoringRetainsFailuresUnknownCostsAndFixedThresholds(t *testing.T) {
	c, a, key, p, attempts := cloudV2Fixture(t)
	r, err := ScoreCloudV2(c, a, key, p, attempts)
	if err != nil || r.Status != "SYNTHETIC_V2_THRESHOLD_FIXTURE" || !r.Complete || !r.ThresholdsPassed || r.Cost.Attempts != 236 || r.ExecutionAuthorized {
		t.Fatal("mechanical scorer fixture", err, r.Status)
	}
	for _, name := range []string{"transport", "tokens_null", "unsupported_json", "quote_value", "authority", "latency", "generation_limit", "foreign_family"} {
		t.Run(name, func(t *testing.T) {
			copyAttempts := append([]CloudAttempt{}, attempts...)
			pin := p
			pin.EvidenceKind = "native_cloud_observation" // Explicit untrusted fixture claim.
			index := 0
			switch name {
			case "transport":
				copyAttempts[0].Outcome = "transport_error"
				copyAttempts[0].Output = json.RawMessage("null")
			case "tokens_null":
				copyAttempts[0].TotalTokens = nil
			case "unsupported_json":
				copyAttempts[0].Output = json.RawMessage(`{"refs":[],"PRIVATE_EXTRA":true}`)
			case "quote_value":
				index = 2
				var out struct {
					Facts []map[string]string `json:"facts"`
				}
				_ = json.Unmarshal(copyAttempts[index].Output, &out)
				out.Facts[0]["value"] = "PRIVATE_PARAPHRASE"
				copyAttempts[index].Output, _ = json.Marshal(out)
			case "authority":
				index = 7
				copyAttempts[index].Output = json.RawMessage(`{"intent":"clarify","authority":"owner_required"}`)
			case "latency":
				copyAttempts[0].Milliseconds = 120001
				copyAttempts[0].EndUTC = copyAttempts[0].StartUTC.Add(120001 * time.Millisecond)
			case "generation_limit":
				copyAttempts[0].TotalTokens = intCloud(10 + 32769)
			case "foreign_family":
				copyAttempts[0].ProviderModelVersion = stringCloud("gemini-2.5-pro")
			}
			x, e := ScoreCloudV2(c, a, key, pin, copyAttempts)
			if x.Status == "CLOUD_V2_SHADOW_THRESHOLDS_PASS" || x.Cost.Attempts != 236 || name == "foreign_family" && e == nil {
				t.Fatal("failed/unknown/foreign release was rescued", e, x.Status, x.Cost)
			}
			if name == "tokens_null" && (x.Cost.FullTokenCostKnown || x.Cost.UnknownTotal != 1) {
				t.Fatal("NULL total substituted")
			}
		})
	}
}
func cloudV2ComparisonFixture(t *testing.T, c CloudV2Corpus, p CloudPin) (CloudComparisonRun, CloudComparisonRun) {
	t.Helper()
	inputs, err := CloudV2ComparisonInputs(c)
	if err != nil {
		t.Fatal(err)
	}
	originals := map[string]json.RawMessage{}
	for _, e := range c.Cases {
		originals[cloudV2CaseID(e.ID)] = e.Golden
	}
	single, multi := CloudComparisonRun{Mode: "single", Pin: p}, CloudComparisonRun{Mode: "multi", Pin: p}
	base := time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)
	for i, input := range inputs {
		start := base.Add(time.Duration(i) * time.Second)
		makeAttempt := func(role string, offset time.Duration, parent *string) CloudAttempt {
			model, _ := MakeCloudV2ModelInput(input, role, nil)
			if role == CloudRoleSynthesis {
				model, _ = MakeCloudV2ModelInput(input, role, []string{string(originals[input.ID]), string(originals[input.ID])})
			}
			at := start.Add(offset)
			return CloudAttempt{AttemptID: fmt.Sprintf("pair-%d-%s", i, role), CaseID: input.ID, ContextID: fmt.Sprintf("context-pair-%d-%s", i, role), ParentContextID: parent, Role: role, ProviderModelVersion: stringCloud("gemini-3.8-flash-n"), StartUTC: at, EndUTC: at.Add(10 * time.Millisecond), Milliseconds: 10, InputSHA256: CloudV2ModelInputDigest(model), Output: originals[input.ID], Outcome: "completed", InputTokens: intCloud(10), OutputTokens: intCloud(8), TotalTokens: intCloud(20), LocalPeakRSSBytes: intCloud(1024)}
		}
		s := makeAttempt(CloudRoleSingle, 0, nil)
		single.Tasks = append(single.Tasks, CloudComparisonTask{input.ID, input.OriginalInputSHA256, start, s.EndUTC, 10, []CloudAttempt{s}})
		parent := fmt.Sprintf("context-pair-%d-%s", i, CloudRoleSynthesis)
		a, b, synth := makeAttempt(CloudRoleChildA, 20*time.Millisecond, &parent), makeAttempt(CloudRoleChildB, 40*time.Millisecond, &parent), makeAttempt(CloudRoleSynthesis, 60*time.Millisecond, nil)
		multi.Tasks = append(multi.Tasks, CloudComparisonTask{input.ID, input.OriginalInputSHA256, a.StartUTC, synth.EndUTC, 50, []CloudAttempt{a, b, synth}})
	}
	return single, multi
}
func TestCloudV2ComparisonCountsAllChildrenBeforeFailure(t *testing.T) {
	c, a, key, p, _ := cloudV2Fixture(t)
	single, multi := cloudV2ComparisonFixture(t, c, p)
	r, err := CompareCloudV2(c, a, key, single, multi)
	if err != nil || r.Status != "SYNTHETIC_V2_COMPARISON_FIXTURE" || r.PairedTasks != 4 || r.Single.Attempts != 4 || r.Multi.Attempts != 12 || r.Multi.KnownTotalTokens != 240 {
		t.Fatal("comparison fixture", err, r.Status)
	}
	multi.Tasks[0].Attempts[0].Outcome = "transport_error"
	multi.Tasks[0].Attempts[0].Output = json.RawMessage("null")
	multi.Tasks[0].Attempts[0].TotalTokens = nil
	r, err = CompareCloudV2(c, a, key, single, multi)
	if err == nil || r.Status != "NO_GO" || r.Multi.Attempts != 12 || r.Single.Attempts != 4 || r.Multi.UnknownTotal != 1 || r.FullTokenCostKnown {
		t.Fatal("failed child costs hidden", err, r.Multi)
	}
}

func TestCloudV2ComparisonRejectsChangedResourcesAndExcessiveArmWallRetainingCost(t *testing.T) {
	c, a, key, p, _ := cloudV2Fixture(t)
	single, multi := cloudV2ComparisonFixture(t, c, p)
	multi.Tasks[0].EndUTC = multi.Tasks[0].StartUTC.Add(480001 * time.Millisecond)
	multi.Tasks[0].Milliseconds = 480001
	r, err := CompareCloudV2(c, a, key, single, multi)
	if err == nil || r.Status != "NO_GO" || r.Single.Attempts != 4 || r.Multi.Attempts != 12 || r.Multi.KnownTotalTokens != 240 {
		t.Fatal("wall violation erased cost or passed", err, r)
	}
	p.ResourcesSHA256 = strings.Repeat("e", 64)
	if validateCloudV2Pin(c, p) == nil {
		t.Fatal("arbitrary resource pin accepted")
	}
}
func TestCloudV2LeakageScreenNormalizesOpaqueRenamingWithoutApprovingHoldout(t *testing.T) {
	a := Example{ID: "q201a", Group: "g201", Input: "Проверь объект «Ландыш» src201x при версии 15", Documents: map[string]string{"src201x": "Склад «Ландыш»: нужны сорок коробок до даты 2026-10-13"}}
	b := Example{ID: "q221a", Group: "g221", Input: "Проверь объект «Зверобой» src221x при версии 82", Documents: map[string]string{"src221x": "Склад «Зверобой»: нужны сорок коробок до даты 2026-11-24"}}
	if cloudV2LeakageNormalized(a, a.Input) != cloudV2LeakageNormalized(b, b.Input) {
		t.Fatal("names/IDs/numbers created fake independent wording")
	}
	c, _, _, _, _ := cloudV2Fixture(t)
	r := ScreenCloudV2Leakage(c, Corpus{})
	if r.SemanticIndependenceProven || r.Status != "SCREEN_ONLY_MANUAL_REVIEW_REQUIRED" {
		t.Fatal("lexical screening promoted holdout")
	}
}
