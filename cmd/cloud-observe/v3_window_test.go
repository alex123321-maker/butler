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

// Completely synthetic CLI frame validation, not native collector evidence.
func TestCloudV3CLIWindowAuditRetains248HistoryAndDeniesForeignPreparedPins(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "source")
	if os.MkdirAll(filepath.Join(root, "testdata", "acceptance"), 0700) != nil {
		t.Fatal("fixture source")
	}
	c := v3CLICorpusFixture(t)
	corpus, _ := json.Marshal(c)
	if os.WriteFile(filepath.Join(root, acceptance.CloudV3CorpusPath), corpus, 0600) != nil {
		t.Fatal("fixture corpus")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptance.GoldenApproval{Digest: acceptance.CloudV3CorpusDigest(c), CriteriaDigest: acceptance.CloudV3CriteriaDigest(c), Rules: acceptance.CloudV3Protocol, Reviewer: "unit-independent-frame-reviewer", CandidateAuthor: "unit-frame-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, unsigned))
	approval, _ := json.Marshal(a)
	key := []byte(hex.EncodeToString(pub) + "\n")
	hardware := []byte(`{"client":"synthetic-frame-only"}`)
	keyPath, approvalPath, hardwarePath, reportPath := filepath.Join(top, "key"), filepath.Join(top, "approval"), filepath.Join(top, "hardware"), filepath.Join(top, "report")
	for path, b := range map[string][]byte{keyPath: key, approvalPath: approval, hardwarePath: hardware} {
		if os.WriteFile(path, b, 0600) != nil {
			t.Fatal("fixture file")
		}
	}
	source, err := acceptance.TreeHash(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	pin := acceptance.CloudPin{EvidenceKind: "native_cloud_observation", Provider: acceptance.CloudProvider, RequestedModel: acceptance.CloudRequestedModel, WeightsStatus: "unavailable", Runtime: "go1.27.2/race; sha256:" + strings.Repeat("a", 64), RuntimeSHA256: strings.Repeat("a", 64), CodeCommit: strings.Repeat("b", 40), SourceSHA256: source, HardwareSHA256: hashCloudV3File(hardware), CorpusDigest: acceptance.CloudV3CorpusDigest(c), SchemaSHA256: acceptance.CloudV3SchemaDigest(), PolicySHA256: acceptance.CloudV3PolicyDigest(c), ResourcesSHA256: acceptance.CloudV3ResourcesDigest(), WindowID: acceptance.CloudV3AuditedWindow, Mode: "shadow_observation"}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4}
	artifacts := []map[string]any{}
	shas := []string{}
	for i, n := range counts {
		hash := hashCloudV3File([]byte(fmt.Sprintf("synthetic-old-%d", i)))
		artifacts = append(artifacts, map[string]any{"report_sha256": hash, "release_ref": "", "new_attempts": n})
		shas = append(shas, hash)
	}
	first := hashCloudV3File([]byte(strings.Join(shas[:4], "\n") + "\n"))
	prior45 := hashCloudV3File([]byte(first + "\n" + strings.Join(shas[4:9], "\n") + "\n"))
	for _, hash := range shas[9:] {
		prior45 = hashCloudV3File([]byte(prior45 + "\n" + hash + "\n"))
	}
	prior228 := hashCloudV3File([]byte(prior45 + "\n" + acceptance.CloudV2PreviousPrivateSHA256 + "\n" + acceptance.CloudV2PreviousAuditSHA256 + "\n"))
	artifacts = append(artifacts, map[string]any{"report_sha256": acceptance.CloudV2PreviousAuditSHA256, "release_ref": acceptance.CloudAuditedCode, "new_attempts": 183})
	baseline := hashCloudV3File([]byte(prior228 + "\n" + acceptance.CloudV3PreviousPrivateSHA256 + "\n" + acceptance.CloudV3PreviousAuditSHA256 + "\n" + acceptance.CloudV3PreviousPGSHA256 + "\n" + acceptance.CloudV3PreviousCorrelationSHA256 + "\n"))
	artifacts = append(artifacts, map[string]any{"report_sha256": acceptance.CloudV3PreviousAuditSHA256, "release_ref": acceptance.CloudV3PreviousCode, "new_attempts": 20})
	usage := []map[string]any{}
	for i := 0; i < 248; i++ {
		u := map[string]any{"run_id": "", "reservation_id": "", "actual_input_tokens": nil, "actual_output_tokens": nil}
		if i < 20 || i >= 45 && i < 247 {
			u["actual_input_tokens"], u["actual_output_tokens"] = 0, 0
		}
		if i >= 45 {
			prefix := "shadow-v1-native-"
			if i >= 228 {
				prefix = "shadow-v2-native-"
			}
			u["run_id"] = "shadow-" + hashCloudV3File([]byte(fmt.Sprint(i)))[:32]
			u["reservation_id"] = fmt.Sprintf("%s%d", prefix, i+1)
		}
		switch i {
		case 0:
			u["actual_input_tokens"], u["actual_output_tokens"] = 74698, 2210
		case 45:
			u["actual_input_tokens"], u["actual_output_tokens"] = 62707, 3083
		case 228:
			u["actual_input_tokens"], u["actual_output_tokens"] = 7141, 244
		}
		usage = append(usage, u)
	}
	start := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	run := func(mode string) acceptance.CloudComparisonRun {
		return acceptance.CloudComparisonRun{Mode: mode, Pin: pin, Tasks: []acceptance.CloudComparisonTask{}}
	}
	report := map[string]any{"status": "failed", "model": acceptance.CloudRequestedModel, "release_ref": pin.CodeCommit, "historical": map[string]any{"status": "mixed_completed_history", "baseline_sha256": baseline, "artifacts": artifacts, "native_attempts": 248, "known_usage_pairs": 222, "unknown_usage_pairs": 26, "usage": usage}, "native_attempts": 248, "new_native_attempts": 0, "maximum_total_attempts": 500, "maximum_new_attempts": 252, "declared_resources": acceptance.FixedCloudV3Resources(), "pin": pin, "quality_attempts": []any{}, "quality": map[string]any{}, "single": run("single"), "multi": run("multi"), "comparison": map[string]any{}, "charged_attempts": []any{}, "started_at_utc": start, "finished_at_utc": start.Add(time.Second), "window_expires_at_utc": start.Add(75 * time.Minute), "execution_authorized": false}
	raw, _ := json.Marshal(report)
	if os.WriteFile(reportPath, raw, 0600) != nil {
		t.Fatal("fixture report")
	}
	args := []string{"-mode", "v3-window-audit", "-root", root, "-observations", reportPath, "-hardware", hardwarePath, "-commit", pin.CodeCommit, "-expected-source-sha256", source, "-expected-history-sha256", baseline, "-golden-public-key", keyPath, "-golden-approval", approvalPath, "-expected-review-key-sha256", hashCloudV3File(key), "-expected-review-approval-sha256", hashCloudV3File(approval)}
	var out bytes.Buffer
	if err := runCLIForCloudV3Window(args, &out); err != nil {
		t.Fatal("offline V3 frame", err)
	}
	var safe acceptance.CloudV3WindowAudit
	if json.Unmarshal(out.Bytes(), &safe) != nil || safe.Status != "NO_GO" || safe.CarryForwardAttempts != 248 || safe.HistoricalKnown != 222 || safe.HistoricalUnknown != 26 || safe.HistoricalNullTotal != 46 || safe.Execution || safe.Cost.FullTokenCostKnown {
		t.Fatal("false quality or lost old NULL")
	}
	for _, flag := range []string{"-expected-source-sha256", "-expected-history-sha256", "-commit"} {
		bad := append([]string{}, args...)
		for i := range bad {
			if bad[i] == flag {
				bad[i+1] = strings.Repeat("f", len(bad[i+1]))
			}
		}
		out.Reset()
		if runCLIForCloudV3Window(bad, &out) == nil || out.Len() != 0 {
			t.Fatal("foreign prepared pin accepted", flag)
		}
	}
}

// Separate from the local variable named run above.
func runCLIForCloudV3Window(args []string, out *bytes.Buffer) error { return run(args, out) }
