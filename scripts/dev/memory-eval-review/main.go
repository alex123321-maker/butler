// Operator seal of already independently approved bytes; never reviews its own
// data, opens provider credentials or invokes inference.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root := flag.String("source", "/src", "read-only source")
	review := flag.String("review", "/review", "fresh independent directory")
	declSHA := flag.String("declaration-sha256", "", "operator-pinned independent review JSON")
	docSHA := flag.String("document-sha256", "", "operator-pinned independent review document")
	flag.Parse()
	if os.Geteuid() != 10001 || !filepath.IsAbs(*root) || !filepath.IsAbs(*review) {
		return errors.New("operator private identity required")
	}
	raw, err := os.ReadFile(filepath.Join(*root, "testdata/acceptance/reviews/ru-memory-v2-review-20261014.json"))
	if err != nil {
		return err
	}
	actual, err := acceptance.FileHash(filepath.Join(*root, "testdata/acceptance/reviews/ru-memory-v2-review-20261014.json"))
	if err != nil || actual != *declSHA {
		return errors.New("independent declaration differs")
	}
	actual, err = acceptance.FileHash(filepath.Join(*root, "docs/ai/memory-eval-v2-independent-review-20261014.md"))
	if err != nil || actual != *docSHA {
		return errors.New("independent review document differs")
	}
	var d struct {
		Status       string `json:"status"`
		Raw          string `json:"raw_suite_sha256"`
		Suite        string `json:"suite_digest"`
		Criteria     string `json:"criteria_sha256"`
		Reviewer     string `json:"reviewer"`
		Author       string `json:"candidate_author"`
		Rules        string `json:"rules"`
		Calls        int    `json:"reviewer_model_calls"`
		Observations int    `json:"model_observations_supplied_to_this_review"`
		Approved     bool   `json:"approved_for_external_golden_criteria_seal_only"`
	}
	if json.Unmarshal(raw, &d) != nil || d.Status != "PASS_GOLDEN_REVIEW_ONLY" || !d.Approved || d.Calls != 0 || d.Observations != 0 || d.Author == d.Reviewer || d.Reviewer == "" {
		return errors.New("independent semantic approval unavailable")
	}
	suiteRaw, err := os.ReadFile(filepath.Join(*root, "testdata/acceptance/ru-memory-v2.json"))
	if err != nil {
		return err
	}
	actual, _ = acceptance.FileHash(filepath.Join(*root, "testdata/acceptance/ru-memory-v2.json"))
	s, err := acceptance.LoadMemorySuite(strings.NewReader(string(suiteRaw)))
	if err != nil || actual != d.Raw || acceptance.MemorySuiteDigest(s) != d.Suite || acceptance.MemoryVersionCriteriaDigest(s) != d.Criteria || s.Version != d.Rules {
		return errors.New("reviewed data/compiled criteria changed")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	a := acceptance.GoldenApproval{Digest: d.Suite, CriteriaDigest: d.Criteria, Rules: d.Rules, Reviewer: d.Reviewer, CandidateAuthor: d.Author}
	unsigned, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, unsigned))
	if err = acceptance.VerifyMemoryGolden(s, a, pub); err != nil {
		return err
	}
	encoded, _ := json.Marshal(a)
	for _, file := range []struct {
		Name string
		Raw  []byte
	}{{"approval.json", append(encoded, '\n')}, {"public-key.hex", []byte(hex.EncodeToString(pub) + "\n")}, {"independent-declaration.json", raw}, {"frozen-suite.json", suiteRaw}} {
		f, err := os.OpenFile(filepath.Join(*review, file.Name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(file.Raw)
		syncErr := f.Sync()
		closeErr := f.Close()
		if err != nil || syncErr != nil || closeErr != nil {
			return errors.New("exclusive seal persistence failed")
		}
	}
	// Private signing bytes are not persisted and cannot approve later changes.
	fmt.Println("independent reviewed bytes sealed; zero inference")
	return nil
}
