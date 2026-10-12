package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

// A correctly signed new dataset cannot reuse a sealed old window's baseline.
func TestCloudV4CLIWindowAuditRejectsOldBaselineWithoutExport(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "source")
	if os.MkdirAll(filepath.Join(root, "testdata", "acceptance"), 0700) != nil {
		t.Fatal("fixture source")
	}
	c := v4CLICorpusFixture(t)
	corpus, _ := json.Marshal(c)
	if os.WriteFile(filepath.Join(root, acceptance.CloudV4CorpusPath), corpus, 0600) != nil {
		t.Fatal("fixture corpus")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptance.GoldenApproval{Digest: acceptance.CloudV4CorpusDigest(c), CriteriaDigest: acceptance.CloudV4CriteriaDigest(c), Rules: acceptance.CloudV4Protocol, Reviewer: "unit-independent-reviewer", CandidateAuthor: "unit-author"}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, unsigned))
	approval, _ := json.Marshal(a)
	key := []byte(hex.EncodeToString(pub) + "\n")
	hardware := []byte(`{"client":"synthetic-only"}`)
	keyPath, approvalPath, hardwarePath, reportPath := filepath.Join(top, "key"), filepath.Join(top, "approval"), filepath.Join(top, "hardware"), filepath.Join(top, "report")
	for path, b := range map[string][]byte{keyPath: key, approvalPath: approval, hardwarePath: hardware, reportPath: []byte(`{"native_attempts":248}`)} {
		if os.WriteFile(path, b, 0600) != nil {
			t.Fatal("fixture artifact")
		}
	}
	args := []string{"-mode", "v4-window-audit", "-root", root, "-observations", reportPath, "-hardware", hardwarePath, "-commit", strings.Repeat("b", 40), "-expected-source-sha256", strings.Repeat("c", 64), "-expected-history-sha256", acceptance.CloudV4HistorySHA256, "-golden-public-key", keyPath, "-golden-approval", approvalPath, "-expected-review-key-sha256", hashCloudV4File(key), "-expected-review-approval-sha256", hashCloudV4File(approval)}
	var out bytes.Buffer
	if run(args, &out) == nil || out.Len() != 0 {
		t.Fatal("sealed old baseline accepted or exported")
	}
}
