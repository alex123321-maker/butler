package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

// CLI fixture only; not the independently authored RU corpus or its approval.
func v2CLICorpusFixture(t *testing.T) acceptance.CloudV2Corpus {
	t.Helper()
	c := acceptance.CloudV2Corpus{Version: acceptance.CloudV2CorpusVersion, ComparisonCaseIDs: []string{"q201a", "q202c", "q203e", "q204g"}}
	for i := 0; i < 30; i++ {
		g := fmt.Sprintf("g%d", 201+i)
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
		c.Groups = append(c.Groups, acceptance.CloudV2Group{ID: g, Split: split, SemanticFamily: g, AttackFamily: g, Provenance: origin})
		for j, kind := range []string{"retrieval", "retrieval", "extraction", "extraction", "classification", "classification", "commands", "commands"} {
			id := fmt.Sprintf("q%d%c", 201+i, 'a'+j)
			doc := "src" + g
			value := hashCloudV2File([]byte(g))
			e := acceptance.CloudV2Case{ID: id, Group: g, Kind: kind, Input: kind + " " + hashCloudV2File([]byte(id)), Documents: map[string]string{}}
			switch kind {
			case "retrieval":
				e.Documents[doc] = value
				refs := []string{}
				if j == 0 {
					refs = append(refs, doc)
				}
				e.Golden, _ = json.Marshal(map[string]any{"refs": refs})
			case "extraction":
				e.Documents[doc] = value
				facts := []any{}
				if j == 2 {
					facts = append(facts, map[string]string{"key": "field", "value": value, "quote": value, "source_id": doc})
				}
				e.Golden, _ = json.Marshal(map[string]any{"facts": facts})
			case "classification":
				label := "important"
				if j == 5 {
					label = "observe"
				}
				e.Golden, _ = json.Marshal(map[string]string{"label": label})
			case "commands":
				intent, authority := "stop_tree", "owner_required"
				if j == 7 {
					intent, authority = "clarify", "none"
				}
				e.Golden, _ = json.Marshal(map[string]string{"intent": intent, "authority": authority})
			}
			e.NoModel = i == 29 && len(e.Documents) > 0
			c.Cases = append(c.Cases, e)
		}
	}
	return c
}

func TestCloudV2CLIWindowAuditRetainsFailedEmptyWindowAndDeniesForeignPreparedPins(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "source")
	if os.MkdirAll(filepath.Join(root, "testdata", "acceptance"), 0700) != nil {
		t.Fatal("fixture source")
	}
	c := v2CLICorpusFixture(t)
	corpus, _ := json.Marshal(c)
	if os.WriteFile(filepath.Join(root, acceptance.CloudV2CorpusPath), corpus, 0600) != nil {
		t.Fatal("fixture corpus")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptance.GoldenApproval{Digest: acceptance.CloudV2CorpusDigest(c), CriteriaDigest: acceptance.CloudV2CriteriaDigest(c), Rules: acceptance.CloudV2Protocol, Reviewer: "audit-CLI-unit-reviewer", CandidateAuthor: "audit-CLI-unit-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, unsigned))
	approval, _ := json.Marshal(a)
	key := []byte(hex.EncodeToString(pub) + "\n")
	hardware := []byte(`{"client":"offline-unit-only"}`)
	keyPath, approvalPath, hardwarePath, reportPath := filepath.Join(top, "key"), filepath.Join(top, "approval"), filepath.Join(top, "hardware"), filepath.Join(top, "report")
	for path, b := range map[string][]byte{keyPath: key, approvalPath: approval, hardwarePath: hardware} {
		if os.WriteFile(path, b, 0600) != nil {
			t.Fatal("fixture metadata")
		}
	}
	source, err := acceptance.TreeHash(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	pin := acceptance.CloudPin{EvidenceKind: "native_cloud_observation", Provider: acceptance.CloudProvider, RequestedModel: acceptance.CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "go1.27.2/race; sha256:" + strings.Repeat("a", 64), RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: source, HardwareSHA256: hashCloudV2File(hardware), CorpusDigest: acceptance.CloudV2CorpusDigest(c), SchemaSHA256: acceptance.CloudV2SchemaDigest(), PolicySHA256: acceptance.CloudV2PolicyDigest(c), ResourcesSHA256: acceptance.CloudV2ResourcesDigest(), WindowID: acceptance.CloudV2AuditedWindow, Mode: "shadow_observation"}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4}
	artifacts, shas := []map[string]any{}, []string{}
	for i, n := range counts {
		hash := hashCloudV2File([]byte(fmt.Sprintf("synthetic-history-%d", i)))
		artifacts = append(artifacts, map[string]any{"report_sha256": hash, "release_ref": "", "new_attempts": n})
		shas = append(shas, hash)
	}
	first := hashCloudV2File([]byte(strings.Join(shas[:4], "\n") + "\n"))
	base := hashCloudV2File([]byte(first + "\n" + strings.Join(shas[4:9], "\n") + "\n"))
	for _, hash := range shas[9:] {
		base = hashCloudV2File([]byte(base + "\n" + hash + "\n"))
	}
	history := hashCloudV2File([]byte(base + "\n" + acceptance.CloudV2PreviousPrivateSHA256 + "\n" + acceptance.CloudV2PreviousAuditSHA256 + "\n"))
	artifacts = append(artifacts, map[string]any{"report_sha256": acceptance.CloudV2PreviousAuditSHA256, "release_ref": acceptance.CloudAuditedCode, "new_attempts": 183})
	usage := []map[string]any{}
	for i := 0; i < 228; i++ {
		u := map[string]any{"run_id": "", "reservation_id": "", "actual_input_tokens": nil, "actual_output_tokens": nil}
		if i < 20 || i >= 45 {
			u["actual_input_tokens"], u["actual_output_tokens"] = 0, 0
		}
		if i == 0 {
			u["actual_input_tokens"], u["actual_output_tokens"] = 74698, 2210
		}
		if i >= 45 {
			u["run_id"], u["reservation_id"] = "shadow-"+hashCloudV2File([]byte(fmt.Sprint(i)))[:32], fmt.Sprintf("shadow-v1-native-%d", i+1)
		}
		if i == 45 {
			u["actual_input_tokens"], u["actual_output_tokens"] = 62707, 3083
		}
		usage = append(usage, u)
	}
	start := time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)
	report := map[string]any{"status": "failed", "model": acceptance.CloudRequestedModel, "release_ref": pin.CodeCommit, "historical": map[string]any{"status": "mixed_completed_history", "baseline_sha256": history, "artifacts": artifacts, "native_attempts": 228, "known_usage_pairs": 203, "unknown_usage_pairs": 25, "usage": usage}, "native_attempts": 228, "new_native_attempts": 0, "maximum_total_attempts": 480, "maximum_new_attempts": 252, "declared_resources": acceptance.FixedCloudV2Resources(), "pin": pin, "quality_attempts": []any{}, "quality": map[string]any{"PRIVATE_STORED_LABEL": "ignored"}, "single": acceptance.CloudComparisonRun{Mode: "single", Pin: pin, Tasks: []acceptance.CloudComparisonTask{}}, "multi": acceptance.CloudComparisonRun{Mode: "multi", Pin: pin, Tasks: []acceptance.CloudComparisonTask{}}, "comparison": map[string]any{}, "charged_attempts": []any{}, "started_at_utc": start, "finished_at_utc": start.Add(time.Second), "window_expires_at_utc": start.Add(75 * time.Minute), "execution_authorized": false}
	raw, _ := json.Marshal(report)
	if os.WriteFile(reportPath, raw, 0600) != nil {
		t.Fatal("fixture report")
	}
	args := []string{"-mode", "v2-window-audit", "-root", root, "-observations", reportPath, "-hardware", hardwarePath, "-commit", pin.CodeCommit, "-golden-public-key", keyPath, "-golden-approval", approvalPath, "-expected-review-key-sha256", hashCloudV2File(key), "-expected-review-approval-sha256", hashCloudV2File(approval), "-expected-source-sha256", source, "-expected-history-sha256", history}
	var output bytes.Buffer
	if err := run(args, &output); err != nil || !strings.Contains(output.String(), `"carry_forward_attempts": 228`) || !strings.Contains(output.String(), `"status": "NO_GO"`) || strings.Contains(output.String(), "PRIVATE_STORED_LABEL") || strings.Contains(output.String(), `"quality_attempts"`) {
		t.Fatal("safe empty/failed audit failed", err)
	}
	for _, flag := range []string{"-commit", "-expected-source-sha256", "-expected-history-sha256"} {
		bad := append([]string{}, args...)
		for i := range bad {
			if bad[i] == flag {
				bad[i+1] = strings.Repeat("f", len(bad[i+1]))
			}
		}
		output.Reset()
		if run(bad, &output) == nil || output.Len() != 0 {
			t.Fatal("foreign prepared audit pin accepted", flag)
		}
	}
}
func TestCloudV2CLIRequiresIndependentReviewAndExactReleaseFiles(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "source")
	if err := os.MkdirAll(filepath.Join(root, "testdata", "acceptance"), 0700); err != nil {
		t.Fatal(err)
	}
	c := v2CLICorpusFixture(t)
	corpusFile, _ := json.Marshal(c)
	if err := os.WriteFile(filepath.Join(root, acceptance.CloudV2CorpusPath), corpusFile, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"-mode", "v2-inputs", "-root", root}, &output); err != nil || strings.Contains(output.String(), `"golden"`) || strings.Contains(output.String(), `"semantic_family"`) {
		t.Fatal("opaque CLI export", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptance.GoldenApproval{Digest: acceptance.CloudV2CorpusDigest(c), CriteriaDigest: acceptance.CloudV2CriteriaDigest(c), Rules: acceptance.CloudV2Protocol, Reviewer: "CLI-unit-reviewer", CandidateAuthor: "CLI-unit-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, unsigned))
	approvalFile, _ := json.Marshal(a)
	keyFile := []byte(hex.EncodeToString(pub) + "\n")
	keyPath, approvalPath := filepath.Join(top, "key.hex"), filepath.Join(top, "approval.json")
	hardwarePath, observationsPath := filepath.Join(top, "hardware.json"), filepath.Join(top, "observations.json")
	hardware := []byte(`{"client":"offline-unit-fixture"}`)
	for path, body := range map[string][]byte{keyPath: keyFile, approvalPath: approvalFile, hardwarePath: hardware} {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	sourceSHA, err := acceptance.TreeHash(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := acceptance.CloudPin{EvidenceKind: "synthetic_fixture", Provider: acceptance.CloudProvider, RequestedModel: acceptance.CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "CLI-unit", RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: sourceSHA, HardwareSHA256: hashCloudV2File(hardware), CorpusDigest: acceptance.CloudV2CorpusDigest(c), SchemaSHA256: acceptance.CloudV2SchemaDigest(), PolicySHA256: acceptance.CloudV2PolicyDigest(c), ResourcesSHA256: acceptance.CloudV2ResourcesDigest(), WindowID: "unit-no-native", Mode: "shadow_observation"}
	observationFile, _ := json.Marshal(struct {
		Pin      acceptance.CloudPin       `json:"pin"`
		Attempts []acceptance.CloudAttempt `json:"attempts"`
	}{p, []acceptance.CloudAttempt{}})
	if err := os.WriteFile(observationsPath, observationFile, 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"-mode", "v2-quality", "-root", root, "-observations", observationsPath, "-hardware", hardwarePath, "-commit", p.CodeCommit, "-golden-public-key", keyPath, "-golden-approval", approvalPath}
	output.Reset()
	if run(base, &output) == nil || output.Len() != 0 {
		t.Fatal("review expectation absent but import succeeded")
	}
	args := append(append([]string{}, base...), "-expected-review-key-sha256", hashCloudV2File(keyFile), "-expected-review-approval-sha256", hashCloudV2File(approvalFile))
	output.Reset()
	if err := run(args, &output); err != nil || !strings.Contains(output.String(), `"status": "NO_GO"`) || strings.Contains(output.String(), a.Signature) {
		t.Fatal("exact offline import", err)
	}
	for _, failure := range []string{"commit", "hardware", "review_key", "source"} {
		t.Run(failure, func(t *testing.T) {
			bad := append([]string{}, args...)
			switch failure {
			case "commit":
				for i := range bad {
					if bad[i] == "-commit" {
						bad[i+1] = strings.Repeat("c", 40)
					}
				}
			case "hardware":
				if err := os.WriteFile(hardwarePath, []byte(`{"client":"foreign"}`), 0600); err != nil {
					t.Fatal(err)
				}
				defer os.WriteFile(hardwarePath, hardware, 0600)
			case "review_key":
				for i := range bad {
					if bad[i] == "-expected-review-key-sha256" {
						bad[i+1] = strings.Repeat("d", 64)
					}
				}
			case "source":
				extra := filepath.Join(root, "foreign.txt")
				if err := os.WriteFile(extra, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
				defer os.Remove(extra)
			}
			var denied bytes.Buffer
			if run(bad, &denied) == nil || denied.Len() != 0 {
				t.Fatal("foreign release accepted")
			}
		})
	}
	artifact := filepath.Join(top, "safe-report.json")
	args = append(args, "-output", artifact)
	if run(args, &output) != nil {
		t.Fatal("new immutable artifact failed")
	}
	before, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if run(args, &output) == nil {
		t.Fatal("existing artifact overwritten")
	}
	after, _ := os.ReadFile(artifact)
	if !bytes.Equal(before, after) {
		t.Fatal("immutable output changed")
	}
}
