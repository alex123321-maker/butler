// cloud-v5-review seals independently approved public benchmark bytes. It never
// imports provider credentials, calls a model or changes the candidate corpus.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/acceptance"
)

func digest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

func writeExclusive(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(raw)
	if err == nil && n != len(raw) {
		err = errors.New("short review write")
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func run() error {
	source := flag.String("source", "/src", "read-only candidate source")
	review := flag.String("review", "/review", "new protected independently owned directory")
	corpusSHA := flag.String("corpus-sha256", "", "independently reviewed exact corpus bytes")
	reviewSHA := flag.String("review-sha256", "", "independently approved exact review declaration")
	semanticSHA := flag.String("semantic-sha256", "", "independently reviewed corpus semantics")
	schemaSHA := flag.String("schema-sha256", "", "independently reviewed schema and instructions")
	policySHA := flag.String("policy-sha256", "", "independently reviewed policy")
	criteriaSHA := flag.String("criteria-sha256", "", "independently reviewed success criteria")
	resourcesSHA := flag.String("resources-sha256", "", "independently reviewed finite resources")
	flag.Parse()
	hash := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if !hash.MatchString(*corpusSHA) || !hash.MatchString(*reviewSHA) || !hash.MatchString(*semanticSHA) || !hash.MatchString(*schemaSHA) || !hash.MatchString(*policySHA) || !hash.MatchString(*criteriaSHA) || !hash.MatchString(*resourcesSHA) || !filepath.IsAbs(*source) || !filepath.IsAbs(*review) || os.Geteuid() != 10001 {
		return errors.New("private exact review identity required")
	}
	stat, err := os.Lstat(*review)
	if err != nil || !stat.IsDir() || stat.Mode().Perm() != 0700 || stat.Mode()&os.ModeSymlink != 0 {
		return errors.New("private review directory required")
	}
	reviewRaw, err := acceptance.ReadPrivateFile(filepath.Join(*review, "independent-review.md"), 256*1024)
	if err != nil || digest(reviewRaw) != *reviewSHA {
		return errors.New("independent review bytes denied")
	}
	raw, err := os.ReadFile(filepath.Join(*source, acceptance.CloudV5CorpusPath))
	if err != nil || len(raw) > 512*1024 || digest(raw) != *corpusSHA {
		return errors.New("independently reviewed corpus bytes denied")
	}
	c, err := acceptance.LoadCloudV5Corpus(strings.NewReader(string(raw)))
	if err != nil {
		return errors.New("reviewed inventory denied")
	}
	if acceptance.CloudV5CorpusDigest(c) != *semanticSHA || acceptance.CloudV5SchemaDigest() != *schemaSHA || acceptance.CloudV5PolicyDigest(c) != *policySHA || acceptance.CloudV5CriteriaDigest(c) != *criteriaSHA || acceptance.CloudV5ResourcesDigest() != *resourcesSHA {
		return errors.New("compiled criteria differ from independently reviewed identities")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return errors.New("review key generation failed")
	}
	approval := acceptance.GoldenApproval{Digest: acceptance.CloudV5CorpusDigest(c), CriteriaDigest: acceptance.CloudV5CriteriaDigest(c), Rules: acceptance.CloudV5Protocol, Reviewer: "independent_audit", CandidateAuthor: "protocol_review"}
	unsigned, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	approval.Signature = hex.EncodeToString(ed25519.Sign(private, unsigned))
	if acceptance.VerifyCloudV5Golden(c, approval, public) != nil {
		return errors.New("review signature verification failed")
	}
	approvalRaw, _ := json.Marshal(approval)
	approvalRaw = append(approvalRaw, '\n')
	publicRaw := []byte(hex.EncodeToString(public) + "\n")
	manifest := struct {
		Version                                                                      string `json:"version"`
		Corpus, Review, Key, Approval, Semantic, Schema, Policy, Criteria, Resources string
		Author, Reviewer                                                             string
		Signature                                                                    string `json:"signature"`
	}{Version: "butler-cloud-v5-independent-review-v1", Corpus: *corpusSHA, Review: *reviewSHA, Key: digest(publicRaw), Approval: digest(approvalRaw), Semantic: acceptance.CloudV5CorpusDigest(c), Schema: acceptance.CloudV5SchemaDigest(), Policy: acceptance.CloudV5PolicyDigest(c), Criteria: acceptance.CloudV5CriteriaDigest(c), Resources: acceptance.CloudV5ResourcesDigest(), Author: approval.CandidateAuthor, Reviewer: approval.Reviewer}
	unsignedManifest, _ := json.Marshal(manifest)
	manifest.Signature = hex.EncodeToString(ed25519.Sign(private, unsignedManifest))
	manifestRaw, _ := json.Marshal(manifest)
	manifestRaw = append(manifestRaw, '\n')
	for _, file := range []struct {
		name string
		raw  []byte
	}{
		{"private-key", private}, {"public-key.hex", publicRaw}, {"approval.json", approvalRaw}, {"frozen-corpus.json", raw}, {"review-manifest.json", manifestRaw},
	} {
		if err := writeExclusive(filepath.Join(*review, file.name), file.raw); err != nil {
			return errors.New("exclusive review persistence failed; retain partial directory")
		}
	}
	return json.NewEncoder(os.Stdout).Encode(struct{ Status, PublicKeySHA256, ApprovalSHA256, ManifestSHA256 string }{"independent_review_sealed", digest(publicRaw), digest(approvalRaw), digest(manifestRaw)})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
