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

	"github.com/butler/butler/internal/acceptance"
)

// CLI fixture only; not the independently authored RU corpus or its approval.
func v5CLICorpusFixture(t *testing.T) acceptance.CloudV5Corpus {
	t.Helper()
	c := acceptance.CloudV5Corpus{Version: acceptance.CloudV5CorpusVersion, ComparisonCaseIDs: []string{"q501a", "q502c", "q503e", "q504g"}}
	for i := 0; i < 30; i++ {
		g := fmt.Sprintf("g%d", 501+i)
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
		c.Groups = append(c.Groups, acceptance.CloudV5Group{ID: g, Split: split, SemanticFamily: g, AttackFamily: g, Provenance: origin})
		for j, kind := range []string{"retrieval", "retrieval", "extraction", "extraction", "classification", "classification", "commands", "commands"} {
			id := fmt.Sprintf("q%d%c", 501+i, 'a'+j)
			doc := "src" + g
			value := hashCloudV5File([]byte(g))
			e := acceptance.CloudV5Case{ID: id, Group: g, Kind: kind, Input: kind + " " + hashCloudV5File([]byte(id)), Documents: map[string]string{}}
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

func TestCloudV5CLIRequiresIndependentReviewAndExactReleaseFiles(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "source")
	if err := os.MkdirAll(filepath.Join(root, "testdata", "acceptance"), 0700); err != nil {
		t.Fatal(err)
	}
	c := v5CLICorpusFixture(t)
	corpusFile, _ := json.Marshal(c)
	if err := os.WriteFile(filepath.Join(root, acceptance.CloudV5CorpusPath), corpusFile, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"-mode", "v5-inputs", "-root", root}, &output); err != nil || strings.Contains(output.String(), `"golden"`) || strings.Contains(output.String(), `"semantic_family"`) {
		t.Fatal("opaque CLI export", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptance.GoldenApproval{Digest: acceptance.CloudV5CorpusDigest(c), CriteriaDigest: acceptance.CloudV5CriteriaDigest(c), Rules: acceptance.CloudV5Protocol, Reviewer: "CLI-unit-reviewer", CandidateAuthor: "CLI-unit-author"}
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
	p := acceptance.CloudPin{EvidenceKind: "synthetic_fixture", Provider: acceptance.CloudProvider, RequestedModel: acceptance.CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "CLI-unit", RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: sourceSHA, HardwareSHA256: hashCloudV5File(hardware), CorpusDigest: acceptance.CloudV5CorpusDigest(c), SchemaSHA256: acceptance.CloudV5SchemaDigest(), PolicySHA256: acceptance.CloudV5PolicyDigest(c), ResourcesSHA256: acceptance.CloudV5ResourcesDigest(), WindowID: "unit-no-native", Mode: "shadow_observation"}
	observationFile, _ := json.Marshal(struct {
		Pin      acceptance.CloudPin       `json:"pin"`
		Attempts []acceptance.CloudAttempt `json:"attempts"`
	}{p, []acceptance.CloudAttempt{}})
	if err := os.WriteFile(observationsPath, observationFile, 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"-mode", "v5-quality", "-root", root, "-observations", observationsPath, "-hardware", hardwarePath, "-commit", p.CodeCommit, "-golden-public-key", keyPath, "-golden-approval", approvalPath}
	output.Reset()
	if run(base, &output) == nil || output.Len() != 0 {
		t.Fatal("review expectation absent but import succeeded")
	}
	args := append(append([]string{}, base...), "-expected-review-key-sha256", hashCloudV5File(keyFile), "-expected-review-approval-sha256", hashCloudV5File(approvalFile))
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
