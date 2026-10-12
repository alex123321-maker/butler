package acceptance

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

const CloudV5Protocol = "ru-cloud-shadow-v5"
const CloudV5CorpusVersion = "butler-ru-cloud-fresh-v5"
const CloudV5CorpusPath = "testdata/acceptance/ru-cloud-fresh-v5.json"
const CloudV5MaximumGeneratedTokens = CloudV4MaximumGeneratedTokens
const CloudV5MaximumVisibleJSONBytes = CloudV4MaximumVisibleJSONBytes
const CloudV5MaximumArrayItems = 32
const CloudV5ExpectedModelVersion = CloudV4ExpectedModelVersion
const CloudV5SchemaDialect = "native_openapi_response_schema_v1"
const CloudV5GenerationSchemaField = "responseSchema"
const CloudV5GoldenPublicKeyFileSHA256 = ""
const CloudV5GoldenApprovalFileSHA256 = ""

// V5 reuses only the immutable shape/witness checks, not V4 data, approval,
// identifiers, hashes, execution window, source or quality verdict.
type CloudV5Group = CloudV4Group
type CloudV5Case = CloudV4Case
type CloudV5Corpus CloudV4Corpus
type CloudV5Resources = CloudV4Resources

func LoadCloudV5Corpus(reader io.Reader) (CloudV5Corpus, error) {
	var c CloudV5Corpus
	raw, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return c, errors.New("bounded fresh prospective corpus required")
	}
	if DecodeStrict(raw, &c, "version", "groups", "comparison_case_ids", "cases") != nil {
		return c, errors.New("fresh prospective corpus schema denied")
	}
	return c, ValidateCloudV5Corpus(c)
}
func ValidateCloudV5Corpus(c CloudV5Corpus) error {
	if c.Version != CloudV5CorpusVersion {
		return errors.New("new V5 corpus identity required")
	}
	x := CloudV4Corpus(c)
	x.Version = CloudV4CorpusVersion
	return ValidateCloudV4Corpus(x)
}
func cloudV5Examples(c CloudV5Corpus) Corpus { return cloudV4Examples(CloudV4Corpus(c)) }
func CloudV5CorpusDigest(c CloudV5Corpus) string {
	return cloudDigest(struct {
		Rules  string
		Corpus CloudV5Corpus
	}{CloudV5Protocol, c})
}
func FrozenCloudV5Policy(c CloudV5Corpus) CloudPolicy {
	p := FrozenCloudPolicy()
	p.Version = CloudV5Protocol
	p.ComparisonOriginalIDs = append([]string{}, c.ComparisonCaseIDs...)
	return p
}
func CloudV5PolicyDigest(c CloudV5Corpus) string { return cloudDigest(FrozenCloudV5Policy(c)) }
func FixedCloudV5Resources() CloudV5Resources    { return FixedCloudV4Resources() }
func CloudV5ResourcesDigest() string {
	return cloudDigest(struct {
		Protocol, Dialect string
		Resources         CloudV5Resources
	}{CloudV5Protocol, CloudV5SchemaDialect, FixedCloudV5Resources()})
}

func CloudV5Instructions() map[string]string {
	x := CloudV4Instructions()
	for kind, instruction := range x {
		x[kind] = instruction + ` Строгий формат финального ответа: один JSON-объект, без Markdown, без обратных кавычек, без блока кода и без пояснений до или после объекта. Первый значимый символ — {, последний — }. `
	}
	return x
}

// The Google OpenAPI dialect is an explicit prospective producer contract.
// Compatibility must be established in the independently frozen diagnostic;
// conversion never repairs a final or changes original-witness checking.
func CloudV5ResponseSchema(kind string, documents map[string]string) (json.RawMessage, error) {
	raw, err := CloudV4ResponseSchema(kind, documents)
	if err != nil {
		return nil, err
	}
	var original map[string]any
	if json.Unmarshal(raw, &original) != nil {
		return nil, errors.New("prospective producer schema denied")
	}
	var convert func(map[string]any) (map[string]any, error)
	convert = func(in map[string]any) (map[string]any, error) {
		out := map[string]any{}
		for key, value := range in {
			switch key {
			case "type":
				typ, ok := value.(string)
				if !ok || typ != "object" && typ != "array" && typ != "string" {
					return nil, errors.New("unsupported prospective schema type")
				}
				out[key] = strings.ToUpper(typ)
			case "additionalProperties":
				if value != false {
					return nil, errors.New("unsupported prospective schema keyword")
				}
				// Google OpenAPI Schema omits this JSON-Schema-only keyword.
				// The application still denies extra response fields.
			case "properties":
				properties, ok := value.(map[string]any)
				if !ok {
					return nil, errors.New("prospective properties denied")
				}
				translated := map[string]any{}
				for name, child := range properties {
					shape, ok := child.(map[string]any)
					if !ok {
						return nil, errors.New("prospective property denied")
					}
					translated[name], err = convert(shape)
					if err != nil {
						return nil, err
					}
				}
				out[key] = translated
			case "items":
				shape, ok := value.(map[string]any)
				if !ok {
					return nil, errors.New("prospective array item denied")
				}
				out[key], err = convert(shape)
				if err != nil {
					return nil, err
				}
			case "required", "enum", "maxItems":
				if key != "maxItems" {
					out[key] = value
				}
			default:
				return nil, errors.New("unsupported prospective producer schema keyword")
			}
		}
		if out["type"] == "ARRAY" {
			out["maxItems"] = "32"
		}
		return out, nil
	}
	schema, err := convert(original)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(schema)
	return encoded, err
}
func CloudV5SchemaDigest() string {
	schemas := map[string]json.RawMessage{}
	for _, kind := range []string{"retrieval", "extraction", "classification", "commands"} {
		schemas[kind], _ = CloudV5ResponseSchema(kind, nil)
	}
	return cloudDigest(struct {
		Protocol, Dialect, GenerationField, SourceRecipe string
		Schemas                                          map[string]json.RawMessage
		Instructions                                     map[string]string
		DataGuard, SynthesisGuard, MIME                  string
		Generated, Visible                               int
	}{CloudV5Protocol, CloudV5SchemaDialect, CloudV5GenerationSchemaField, "all_original_map_keys_sorted_exact_v1; uppercase_openapi_types; omit_additionalProperties_only; every_array_maxItems_string_32_with_programmatic_cap; full_per_case_schema_digest; no_golden_subset_or_final_repair", schemas, CloudV5Instructions(), cloudDataGuard, cloudSynthesisGuard, "application/json", CloudV5MaximumGeneratedTokens, CloudV5MaximumVisibleJSONBytes})
}
func CloudV5CriteriaDigest(c CloudV5Corpus) string {
	return cloudDigest(struct{ Rules, Schema, Policy, Percentile, ObservedVersion string }{CloudV5Protocol, CloudV5SchemaDigest(), CloudV5PolicyDigest(c), "nearest_rank_ceil_95_percent_v1", CloudV5ExpectedModelVersion})
}
func VerifyCloudV5Response(e Example, raw []byte) CloudResponseCheck {
	x := VerifyCloudV4Response(e, raw)
	if !x.SchemaError && (e.Kind == "retrieval" || e.Kind == "extraction") {
		var arrays map[string][]json.RawMessage
		if json.Unmarshal(raw, &arrays) != nil {
			x.SchemaError = true
		} else {
			for _, array := range arrays {
				if len(array) > CloudV5MaximumArrayItems {
					x.SchemaError = true
				}
			}
		}
	}
	return x
}
func CloudV5OutputDigest(raw []byte) (string, error) { return CloudV4OutputDigest(raw) }
func cloudV5CaseID(id string) string {
	return "case:" + cloudDigest(CloudV5Protocol + "\x00" + id)[:32]
}
func cloudV5Input(e Example) CloudInputCase {
	x := cloudInput(e)
	x.ID = cloudV5CaseID(e.ID)
	x.OriginalInputSHA256 = ""
	x.OriginalInputSHA256 = cloudDigest(x)
	return x
}
func ExportCloudV5Inputs(c CloudV5Corpus, split string) (CloudInputs, error) {
	if err := ValidateCloudV5Corpus(c); err != nil {
		return CloudInputs{}, err
	}
	if split != "" && split != "development" && split != "tuning" && split != "holdout" {
		return CloudInputs{}, errors.New("unknown fresh split")
	}
	x := CloudInputs{Protocol: CloudV5Protocol, Mode: "shadow_observation", CorpusDigest: CloudV5CorpusDigest(c), SchemaSHA256: CloudV5SchemaDigest(), PolicySHA256: CloudV5PolicyDigest(c), Split: split, Cases: []CloudInputCase{}, BlockedIDs: []string{}}
	for _, e := range cloudV5Examples(c).Examples {
		if split != "" && e.Split != split {
			continue
		}
		if e.NoModel {
			x.BlockedIDs = append(x.BlockedIDs, cloudV5CaseID(e.ID))
		} else {
			x.Cases = append(x.Cases, cloudV5Input(e))
		}
	}
	return x, nil
}
func CloudV5ComparisonInputs(c CloudV5Corpus) ([]CloudInputCase, error) {
	if err := ValidateCloudV5Corpus(c); err != nil {
		return nil, err
	}
	byID := map[string]Example{}
	for _, e := range cloudV5Examples(c).Examples {
		byID[e.ID] = e
	}
	x := []CloudInputCase{}
	for _, id := range c.ComparisonCaseIDs {
		x = append(x, cloudV5Input(byID[id]))
	}
	return x, nil
}

type CloudV5ModelInput struct {
	CloudV4ModelInput
	SchemaDialect         string `json:"schema_dialect"`
	GenerationSchemaField string `json:"generation_config_schema_field"`
}

func MakeCloudV5ModelInput(c CloudInputCase, role string, children []string) (CloudV5ModelInput, error) {
	schema, err := CloudV5ResponseSchema(c.Kind, c.Documents)
	if err != nil {
		return CloudV5ModelInput{}, err
	}
	base, err := MakeCloudV4ModelInput(c, role, children)
	if err != nil {
		return CloudV5ModelInput{}, err
	}
	base.Schema = schema
	base.SystemInstructions = cloudDataGuard + "\n" + CloudV5Instructions()[c.Kind]
	if role == CloudRoleSynthesis {
		base.SystemInstructions += "\n" + cloudSynthesisGuard
	}
	return CloudV5ModelInput{base, CloudV5SchemaDialect, CloudV5GenerationSchemaField}, nil
}
func CloudV5ModelInputDigest(x CloudV5ModelInput) string { return cloudDigest(x) }
func VerifyCloudV5Golden(c CloudV5Corpus, a GoldenApproval, key []byte) error {
	if err := ValidateCloudV5Corpus(c); err != nil {
		return err
	}
	if len(key) != ed25519.PublicKeySize || a.Digest != CloudV5CorpusDigest(c) || a.CriteriaDigest != CloudV5CriteriaDigest(c) || a.Rules != CloudV5Protocol || a.Reviewer == "" || a.CandidateAuthor == "" || a.Reviewer == a.CandidateAuthor {
		return errors.New("independent fresh V5 approval required")
	}
	sig, err := hex.DecodeString(a.Signature)
	if err != nil {
		return errors.New("fresh V5 signature denied")
	}
	unsigned := a
	unsigned.Signature = ""
	raw, _ := json.Marshal(unsigned)
	if !ed25519.Verify(key, raw, sig) {
		return errors.New("fresh V5 signature denied")
	}
	return nil
}
func DecodeCloudV5GoldenArtifacts(c CloudV5Corpus, keyFile, approvalFile []byte) (GoldenApproval, []byte, error) {
	return DecodeCloudV5GoldenArtifactsWithPins(c, keyFile, approvalFile, CloudV5GoldenPublicKeyFileSHA256, CloudV5GoldenApprovalFileSHA256)
}
func DecodeCloudV5GoldenArtifactsWithPins(c CloudV5Corpus, keyFile, approvalFile []byte, keySHA, approvalSHA string) (GoldenApproval, []byte, error) {
	var a GoldenApproval
	if !validSHA(keySHA) || !validSHA(approvalSHA) || hashWindowBytes(keyFile) != keySHA || hashWindowBytes(approvalFile) != approvalSHA {
		return a, nil, errors.New("fresh independent review artifact identity denied")
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyFile)))
	if err != nil {
		return a, nil, errors.New("fresh review public key denied")
	}
	if DecodeStrict(approvalFile, &a, "digest", "criteria_sha256", "rules", "reviewer", "candidate_author", "signature") != nil {
		return a, nil, errors.New("fresh review schema denied")
	}
	return a, key, VerifyCloudV5Golden(c, a, key)
}
func validateCloudV5Pin(c CloudV5Corpus, p CloudPin) error {
	if p.Mode != "shadow_observation" || p.EvidenceKind != "native_cloud_observation" && p.EvidenceKind != "synthetic_fixture" || p.Provider != CloudProvider || p.RequestedModel != CloudRequestedModel || p.WeightsStatus != "unavailable" || p.WeightsSHA256 != nil || p.Runtime == "" || p.WindowID == "" || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(p.CodeCommit) || !validSHA(p.SourceSHA256) || !validSHA(p.RuntimeSHA256) || !validSHA(p.HardwareSHA256) || p.CorpusDigest != CloudV5CorpusDigest(c) || p.SchemaSHA256 != CloudV5SchemaDigest() || p.PolicySHA256 != CloudV5PolicyDigest(c) || p.ResourcesSHA256 != CloudV5ResourcesDigest() || p.ProviderModelVersion != nil && *p.ProviderModelVersion != CloudV5ExpectedModelVersion {
		return errors.New("fresh prospective release pins denied")
	}
	return nil
}
