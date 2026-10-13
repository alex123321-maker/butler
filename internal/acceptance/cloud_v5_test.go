package acceptance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Mechanical fixture only. These random-token templates are deliberately not
// the authored Russian corpus, an independent holdout or model-quality evidence.
func cloudV5Fixture(t *testing.T) (CloudV5Corpus, GoldenApproval, []byte, CloudPin, []CloudAttempt) {
	t.Helper()
	c := CloudV5Corpus{Version: CloudV5CorpusVersion, ComparisonCaseIDs: []string{"q501a", "q502c", "q503e", "q504g"}}
	for i := 0; i < 30; i++ {
		group := fmt.Sprintf("g%d", 501+i)
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
		c.Groups = append(c.Groups, CloudV5Group{group, split, "semantic-" + group, "mechanism-" + group, origin})
		for j, kind := range []string{"retrieval", "retrieval", "extraction", "extraction", "classification", "classification", "commands", "commands"} {
			id := fmt.Sprintf("q%d%c", 501+i, 'a'+j)
			doc := fmt.Sprintf("src%dx", 501+i)
			value := "VALUE-" + cloudDigest(group)[:24]
			e := CloudV5Case{ID: id, Group: group, Kind: kind, Input: kind + " " + cloudDigest(id), Documents: map[string]string{}}
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
	if err := ValidateCloudV5Corpus(c); err != nil {
		t.Fatal("mechanical fixture invalid", err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := GoldenApproval{Digest: CloudV5CorpusDigest(c), CriteriaDigest: CloudV5CriteriaDigest(c), Rules: CloudV5Protocol, Reviewer: "independent-unit-fixture", CandidateAuthor: "unit-fixture-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(private, unsigned))
	p := CloudPin{EvidenceKind: "synthetic_fixture", Provider: CloudProvider, RequestedModel: CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "unit-only", RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: strings.Repeat("c", 64), HardwareSHA256: strings.Repeat("d", 64), CorpusDigest: CloudV5CorpusDigest(c), SchemaSHA256: CloudV5SchemaDigest(), PolicySHA256: CloudV5PolicyDigest(c), ResourcesSHA256: CloudV5ResourcesDigest(), WindowID: "unit-fixture-never-native", Mode: "shadow_observation"}
	inputs, err := ExportCloudV5Inputs(c, "")
	if err != nil {
		t.Fatal(err)
	}
	gold := map[string]json.RawMessage{}
	for _, e := range c.Cases {
		gold[cloudV5CaseID(e.ID)] = e.Golden
	}
	attempts := []CloudAttempt{}
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for i, input := range inputs.Cases {
		model, _ := MakeCloudV5ModelInput(input, CloudRoleSingle, nil)
		start := at.Add(time.Duration(i) * time.Second)
		attempts = append(attempts, CloudAttempt{AttemptID: fmt.Sprintf("fixture-%d", i), CaseID: input.ID, ContextID: fmt.Sprintf("context-%d", i), Role: CloudRoleSingle, ProviderModelVersion: stringCloud("gemini-3.8-flash-n"), StartUTC: start, EndUTC: start.Add(10 * time.Millisecond), Milliseconds: 10, InputSHA256: CloudV5ModelInputDigest(model), Output: gold[input.ID], Outcome: "completed", InputTokens: intCloud(10), OutputTokens: intCloud(8), TotalTokens: intCloud(20), LocalPeakRSSBytes: intCloud(1024)})
	}
	return c, a, public, p, attempts
}
func TestCloudV5ExplicitCorpusAndOpaqueInputContract(t *testing.T) {
	c, _, _, _, _ := cloudV5Fixture(t)
	raw, _ := json.Marshal(c)
	loaded, err := LoadCloudV5Corpus(bytes.NewReader(raw))
	if err != nil || CloudV5CorpusDigest(loaded) != CloudV5CorpusDigest(c) {
		t.Fatal("explicit reader", err)
	}
	x, err := ExportCloudV5Inputs(c, "")
	if err != nil || len(x.Cases) != 236 || len(x.BlockedIDs) != 4 || x.Protocol != CloudV5Protocol {
		t.Fatal("secret gate", err)
	}
	encoded, _ := json.Marshal(x)
	for _, forbidden := range []string{`"golden"`, `"group"`, `"attack_family"`, `"semantic_family"`, cloudV5CaseID("q530a") + `","kind"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatal("metadata/blocked case leaked")
		}
	}
	input := x.Cases[0]
	model, err := MakeCloudV5ModelInput(input, CloudRoleSingle, nil)
	if err != nil || model.ResponseMIMEType != "application/json" || model.MaximumGeneratedTokens != 32768 || len(model.Schema) > 16384 || bytes.Contains(model.Schema, []byte("$ref")) || bytes.Contains(model.Schema, []byte(input.ID)) {
		t.Fatal("original-map native contract", err)
	}
	changed := model
	changed.MaximumGeneratedTokens = 8192
	if CloudV5ModelInputDigest(changed) == CloudV5ModelInputDigest(model) {
		t.Fatal("generation allowance not pinned")
	}
	for _, kind := range []string{"retrieval", "extraction", "classification", "commands"} {
		schema, e := CloudV5ResponseSchema(kind, input.Documents)
		if e != nil || !json.Valid(schema) || bytes.Contains(schema, []byte(`"additionalProperties"`)) || !bytes.Contains(schema, []byte(`"type":"OBJECT"`)) {
			t.Fatal("prospective Google OpenAPI producer schema", kind)
		}
	}
	if model.SchemaDialect != CloudV5SchemaDialect || model.GenerationSchemaField != CloudV5GenerationSchemaField {
		t.Fatal("producer dialect not in input")
	}
	changed = model
	changed.SchemaDialect = "legacy_json_schema"
	if CloudV5ModelInputDigest(changed) == CloudV5ModelInputDigest(model) {
		t.Fatal("different producer dialect not pinned")
	}
	if CloudV5SchemaDigest() == CloudSchemaDigest() || CloudV5PolicyDigest(c) == CloudPolicyDigest() || CloudPolicyDigest() != "20313d5ec68ef865f51fa0c49a10f8ae9443a66f1e4e2a51c309a1d67073fafb" || CloudSchemaDigest() != "a0009cc050b4f46f5efdc84efec67bcbc07dd3d18add61c34e28ccb330e422f2" {
		t.Fatal("V1 boundary changed")
	}
}
func TestCloudV5ReviewAndStrictCorpusDenials(t *testing.T) {
	c, a, key, _, _ := cloudV5Fixture(t)
	if VerifyCloudV5Golden(c, a, key) != nil {
		t.Fatal("fixture approval")
	}
	if _, _, err := DecodeCloudV5GoldenArtifacts(c, nil, nil); err == nil {
		t.Fatal("unreviewed V5 constants acquired approval")
	}
	keyFile := []byte(hex.EncodeToString(key) + "\n")
	approvalFile, _ := json.Marshal(a)
	if _, _, err := DecodeCloudV5GoldenArtifactsWithPins(c, keyFile, approvalFile, hashWindowBytes(keyFile), hashWindowBytes(approvalFile)); err != nil {
		t.Fatal("independent fixture pins", err)
	}
	if _, _, err := DecodeCloudV5GoldenArtifactsWithPins(c, keyFile, approvalFile, CloudGoldenPublicKeyFileSHA256, CloudGoldenApprovalFileSHA256); err == nil {
		t.Fatal("old V1 review reused")
	}
	if _, _, err := DecodeCloudV5GoldenArtifactsWithPins(c, keyFile, approvalFile, CloudV2GoldenPublicKeyFileSHA256, CloudV2GoldenApprovalFileSHA256); err == nil {
		t.Fatal("old V2 review reused")
	}
	raw, _ := json.Marshal(c)
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"version"`), []byte(`"Version"`), 1), bytes.Replace(raw, []byte(`"no_model":false`), []byte(`"no_model":false,"no_model":true`), 1), append(raw, []byte(`{}`)...), bytes.Repeat([]byte("x"), (2<<20)+1)} {
		if _, err := LoadCloudV5Corpus(bytes.NewReader(bad)); err == nil {
			t.Fatal("strict corpus accepted alias/duplicate/trailing/oversize")
		}
	}
	for _, name := range []string{"family_cross_split", "wrong_secret", "comparison_holdout", "nonanchored_gold", "version"} {
		t.Run(name, func(t *testing.T) {
			var bad CloudV5Corpus
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
				bad.Version = "butler-ru-240-v5"
			}
			if ValidateCloudV5Corpus(bad) == nil {
				t.Fatal("invalid prospective inventory accepted")
			}
		})
	}
}
func TestCloudV5ScoringRetainsFailuresUnknownCostsAndFixedThresholds(t *testing.T) {
	c, a, key, p, attempts := cloudV5Fixture(t)
	r, err := ScoreCloudV5(c, a, key, p, attempts)
	if err != nil || r.Status != "SYNTHETIC_V5_THRESHOLD_FIXTURE" || !r.Complete || !r.ThresholdsPassed || r.Cost.Attempts != 236 || r.ExecutionAuthorized {
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
			x, e := ScoreCloudV5(c, a, key, pin, copyAttempts)
			if x.Status == "CLOUD_V5_SHADOW_THRESHOLDS_PASS" || x.Cost.Attempts != 236 || name == "foreign_family" && e == nil {
				t.Fatal("failed/unknown/foreign release was rescued", e, x.Status, x.Cost)
			}
			if name == "tokens_null" && (x.Cost.FullTokenCostKnown || x.Cost.UnknownTotal != 1) {
				t.Fatal("NULL total substituted")
			}
		})
	}
}
func cloudV5ComparisonFixture(t *testing.T, c CloudV5Corpus, p CloudPin) (CloudComparisonRun, CloudComparisonRun) {
	t.Helper()
	inputs, err := CloudV5ComparisonInputs(c)
	if err != nil {
		t.Fatal(err)
	}
	originals := map[string]json.RawMessage{}
	for _, e := range c.Cases {
		originals[cloudV5CaseID(e.ID)] = e.Golden
	}
	single, multi := CloudComparisonRun{Mode: "single", Pin: p}, CloudComparisonRun{Mode: "multi", Pin: p}
	base := time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)
	for i, input := range inputs {
		start := base.Add(time.Duration(i) * time.Second)
		makeAttempt := func(role string, offset time.Duration, parent *string) CloudAttempt {
			model, _ := MakeCloudV5ModelInput(input, role, nil)
			if role == CloudRoleSynthesis {
				model, _ = MakeCloudV5ModelInput(input, role, []string{string(originals[input.ID]), string(originals[input.ID])})
			}
			at := start.Add(offset)
			return CloudAttempt{AttemptID: fmt.Sprintf("pair-%d-%s", i, role), CaseID: input.ID, ContextID: fmt.Sprintf("context-pair-%d-%s", i, role), ParentContextID: parent, Role: role, ProviderModelVersion: stringCloud("gemini-3.8-flash-n"), StartUTC: at, EndUTC: at.Add(10 * time.Millisecond), Milliseconds: 10, InputSHA256: CloudV5ModelInputDigest(model), Output: originals[input.ID], Outcome: "completed", InputTokens: intCloud(10), OutputTokens: intCloud(8), TotalTokens: intCloud(20), LocalPeakRSSBytes: intCloud(1024)}
		}
		s := makeAttempt(CloudRoleSingle, 0, nil)
		single.Tasks = append(single.Tasks, CloudComparisonTask{input.ID, input.OriginalInputSHA256, start, s.EndUTC, 10, []CloudAttempt{s}})
		parent := fmt.Sprintf("context-pair-%d-%s", i, CloudRoleSynthesis)
		a, b, synth := makeAttempt(CloudRoleChildA, 20*time.Millisecond, &parent), makeAttempt(CloudRoleChildB, 40*time.Millisecond, &parent), makeAttempt(CloudRoleSynthesis, 60*time.Millisecond, nil)
		multi.Tasks = append(multi.Tasks, CloudComparisonTask{input.ID, input.OriginalInputSHA256, a.StartUTC, synth.EndUTC, 50, []CloudAttempt{a, b, synth}})
	}
	return single, multi
}
func TestCloudV5ComparisonCountsAllChildrenBeforeFailure(t *testing.T) {
	c, a, key, p, _ := cloudV5Fixture(t)
	single, multi := cloudV5ComparisonFixture(t, c, p)
	r, err := CompareCloudV5(c, a, key, single, multi)
	if err != nil || r.Status != "SYNTHETIC_V5_COMPARISON_FIXTURE" || r.PairedTasks != 4 || r.Single.Attempts != 4 || r.Multi.Attempts != 12 || r.Multi.KnownTotalTokens != 240 {
		t.Fatal("comparison fixture", err, r.Status)
	}
	multi.Tasks[0].Attempts[0].Outcome = "transport_error"
	multi.Tasks[0].Attempts[0].Output = json.RawMessage("null")
	multi.Tasks[0].Attempts[0].TotalTokens = nil
	r, err = CompareCloudV5(c, a, key, single, multi)
	if err == nil || r.Status != "NO_GO" || r.Multi.Attempts != 12 || r.Single.Attempts != 4 || r.Multi.UnknownTotal != 1 || r.FullTokenCostKnown {
		t.Fatal("failed child costs hidden", err, r.Multi)
	}
}

func TestCloudV5ComparisonRejectsChangedResourcesAndExcessiveArmWallRetainingCost(t *testing.T) {
	c, a, key, p, _ := cloudV5Fixture(t)
	single, multi := cloudV5ComparisonFixture(t, c, p)
	multi.Tasks[0].EndUTC = multi.Tasks[0].StartUTC.Add(480001 * time.Millisecond)
	multi.Tasks[0].Milliseconds = 480001
	r, err := CompareCloudV5(c, a, key, single, multi)
	if err == nil || r.Status != "NO_GO" || r.Single.Attempts != 4 || r.Multi.Attempts != 12 || r.Multi.KnownTotalTokens != 240 {
		t.Fatal("wall violation erased cost or passed", err, r)
	}
	p.ResourcesSHA256 = strings.Repeat("e", 64)
	if validateCloudV5Pin(c, p) == nil {
		t.Fatal("arbitrary resource pin accepted")
	}
}
func TestCloudV5LeakageScreenNormalizesOpaqueRenamingWithoutApprovingHoldout(t *testing.T) {
	a := Example{ID: "q501a", Group: "g201", Input: "Проверь объект «Ландыш» src201x при версии 15", Documents: map[string]string{"src201x": "Склад «Ландыш»: нужны сорок коробок до даты 2026-10-13"}}
	b := Example{ID: "q221a", Group: "g221", Input: "Проверь объект «Зверобой» src221x при версии 82", Documents: map[string]string{"src221x": "Склад «Зверобой»: нужны сорок коробок до даты 2026-11-24"}}
	if cloudV5LeakageNormalized(a, a.Input) != cloudV5LeakageNormalized(b, b.Input) {
		t.Fatal("names/IDs/numbers created fake independent wording")
	}
	c, _, _, _, _ := cloudV5Fixture(t)
	r := ScreenCloudV5Leakage(c, Corpus{})
	if r.SemanticIndependenceProven || r.Status != "SCREEN_ONLY_MANUAL_REVIEW_REQUIRED" {
		t.Fatal("lexical screening promoted holdout")
	}
}

func TestCloudV5OutputDigestPinsCanonicalJSONWithoutRepairingModelContent(t *testing.T) {
	a, err := CloudV5OutputDigest([]byte(` { "authority": "none", "intent":"clarify" } `))
	b, secondErr := CloudV5OutputDigest([]byte(`{"intent":"clarify","authority":"none"}`))
	if err != nil || secondErr != nil || a != b {
		t.Fatal("report indentation/key order changed child provenance")
	}
	c, _ := CloudV5OutputDigest([]byte(`{"intent":"clarify","authority":"owner_required"}`))
	if c == a {
		t.Fatal("string content repaired")
	}
	for _, raw := range []string{"```json\n{}\n```", `{"intent":"clarify","intent":"stop_tree"}`, `{"x":1,"\u0078":2}`} {
		if _, err := CloudV5OutputDigest([]byte(raw)); err == nil {
			t.Fatal("ambiguous/Markdown output repaired")
		}
	}
}

func TestCloudV5NearestRankP95Boundary(t *testing.T) {
	c, approval, key, pin, attempts := cloudV5Fixture(t)
	// Tuning has64 cases: rank61 is the first of four values above60s.
	for i := 96; i < 160; i++ {
		ms := int64(60000)
		if i >= 156 {
			ms = 60001
		}
		attempts[i].Milliseconds = ms
		attempts[i].EndUTC = attempts[i].StartUTC.Add(time.Duration(ms) * time.Millisecond)
	}
	r, err := ScoreCloudV5(c, approval, key, pin, attempts)
	if err != nil || r.Splits["tuning"].LatencyP95MS != 60001 || r.ThresholdsPassed {
		t.Fatal("nearest-rank p95 allowed the fourth slow case", err)
	}
	attempts[156].Milliseconds = 60000
	attempts[156].EndUTC = attempts[156].StartUTC.Add(60000 * time.Millisecond)
	r, err = ScoreCloudV5(c, approval, key, pin, attempts)
	if err != nil || r.Splits["tuning"].LatencyP95MS != 60000 || !r.ThresholdsPassed {
		t.Fatal("nearest-rank p95 denied allowed three slow cases", err)
	}
}

func TestCloudV5AuthoredCandidateInventoryPins(t *testing.T) {
	raw, err := os.ReadFile("../../" + CloudV5CorpusPath)
	if err != nil {
		t.Fatal("fresh authored corpus unavailable", err)
	}
	c, err := LoadCloudV5Corpus(bytes.NewReader(raw))
	if err != nil {
		t.Fatal("fresh authored corpus inventory", err)
	}
	t.Logf("raw=%s corpus=%s schema=%s policy=%s criteria=%s resources=%s", hashWindowBytes(raw), CloudV5CorpusDigest(c), CloudV5SchemaDigest(), CloudV5PolicyDigest(c), CloudV5CriteriaDigest(c), CloudV5ResourcesDigest())
}

func TestCloudV5SynthesisRetainsCanonicalChildDataAcrossReportFormatting(t *testing.T) {
	c, _, _, _, _ := cloudV5Fixture(t)
	inputs, err := CloudV5ComparisonInputs(c)
	if err != nil {
		t.Fatal(err)
	}
	compact := `{"intent":"stop_tree","authority":"owner_required"}`
	reformatted := " {\n  \"authority\": \"owner_required\",\n  \"intent\": \"stop_tree\"\n} "
	a, err := MakeCloudV5ModelInput(inputs[3], CloudRoleSynthesis, []string{compact, compact})
	if err != nil {
		t.Fatal(err)
	}
	b, err := MakeCloudV5ModelInput(inputs[3], CloudRoleSynthesis, []string{reformatted, compact})
	if err != nil || CloudV5ModelInputDigest(a) != CloudV5ModelInputDigest(b) {
		t.Fatal("report whitespace/key ordering changed synthesis DATA", err)
	}
	b, err = MakeCloudV5ModelInput(inputs[3], CloudRoleSynthesis, []string{`{"intent":"clarify","authority":"none"}`, compact})
	if err != nil || CloudV5ModelInputDigest(a) == CloudV5ModelInputDigest(b) {
		t.Fatal("different child values were normalized away", err)
	}
}
