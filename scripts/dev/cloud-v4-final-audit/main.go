// cloud-v4-final-audit is a read-only, network-free operator witness checker.
// It prints only hashes, counts and programmatic metrics, never model/source text.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/internal/acceptance"
)

type diagnostic struct {
	Ordinal       int     `json:"ordinal"`
	Context       string  `json:"context_id"`
	Outcome       string  `json:"outcome"`
	Input         *int64  `json:"input_tokens"`
	Output        *int64  `json:"output_tokens"`
	Total         *int64  `json:"total_tokens"`
	Stage         string  `json:"collector_stage"`
	Code          string  `json:"diagnostic_code"`
	Reason        string  `json:"encoding_reason"`
	Bytes         *int    `json:"final_bytes"`
	Hash          *string `json:"final_hash"`
	Retained      bool    `json:"raw_retained"`
	RetentionOK   bool    `json:"retention_ok"`
	Version       *string `json:"model_version"`
	Finish        *string `json:"finish_reason"`
	ProviderStage *string `json:"provider_stage"`
	Finals        int     `json:"final_count"`
	Terminals     int     `json:"terminal_count"`
	Clean         bool    `json:"lifecycle_clean"`
}

func (d *diagnostic) UnmarshalJSON(raw []byte) error {
	type plain diagnostic
	if encodingReason(raw) != "json_object" {
		return errors.New("diagnostic object encoding denied")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return errors.New("diagnostic fields denied")
	}
	// The existing strict decoder requires present non-NULL values. Preserve
	// explicit unknowns while checking the same exact allowlisted inventory.
	for _, name := range []string{"input_tokens", "output_tokens", "total_tokens", "final_bytes", "final_hash", "model_version", "finish_reason", "provider_stage"} {
		if value, exists := fields[name]; exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			fields[name] = json.RawMessage("0")
		}
	}
	patched, _ := json.Marshal(fields)
	var scope map[string]json.RawMessage
	if acceptance.DecodeStrict(patched, &scope, "ordinal", "context_id", "outcome", "input_tokens", "output_tokens", "total_tokens", "collector_stage", "diagnostic_code", "encoding_reason", "final_bytes", "final_hash", "raw_retained", "retention_ok", "model_version", "finish_reason", "provider_stage", "final_count", "terminal_count", "lifecycle_clean") != nil {
		return errors.New("diagnostic scope denied")
	}
	return json.Unmarshal(raw, (*plain)(d))
}

func hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

func readPinned(path, expected string, maximum int64) ([]byte, error) {
	raw, err := acceptance.ReadPrivateFile(path, maximum)
	if err != nil || len(expected) != 64 || hash(raw) != expected {
		return nil, errors.New("exact audit input identity denied")
	}
	return raw, nil
}

// Independently decode structural encoding facts; no repair or schema inference.
func encodingReason(raw []byte) string {
	if len(raw) > 8192 {
		return "visible_bound"
	}
	if !utf8.Valid(raw) {
		return "invalid_utf8"
	}
	if !json.Valid(raw) {
		return "json_syntax"
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return "non_object"
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) string
	visit = func(depth int) string {
		if depth > 16 {
			return "depth_bound"
		}
		t, err := d.Token()
		if err != nil {
			return "json_syntax"
		}
		kind, compound := t.(json.Delim)
		if !compound {
			return "json_object"
		}
		if kind == '{' {
			keys := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				key, ok := k.(string)
				if err != nil || !ok {
					return "json_syntax"
				}
				if keys[key] {
					return "duplicate_key"
				}
				keys[key] = true
				if r := visit(depth + 1); r != "json_object" {
					return r
				}
			}
		} else if kind == '[' {
			n := 0
			for d.More() {
				if n >= 256 {
					return "array_bound"
				}
				n++
				if r := visit(depth + 1); r != "json_object" {
					return r
				}
			}
		} else {
			return "json_syntax"
		}
		if _, err := d.Token(); err != nil {
			return "json_syntax"
		}
		return "json_object"
	}
	return visit(0)
}

func readRaw(path string) ([]byte, error) {
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 || stat.Size() > 8192 {
		return nil, errors.New("private raw identity denied")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("private raw read denied")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return nil, errors.New("private raw identity changed")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || int64(len(raw)) != stat.Size() {
		return nil, errors.New("private raw bytes denied")
	}
	return raw, nil
}

func run() error {
	reportSHA := flag.String("report-sha256", "", "exact private collector identity")
	pgSHA := flag.String("pg-sha256", "", "exact independent attempt witness")
	diagnosticSHA := flag.String("diagnostics-sha256", "", "exact independent diagnostic witness")
	code := flag.String("commit", "", "prepared code commit")
	source := flag.String("source-sha256", "", "prepared source digest")
	hardware := flag.String("hardware-sha256", "", "prepared hardware digest")
	flag.Parse()
	if os.Geteuid() != 10001 {
		return errors.New("private read-only audit owner required")
	}
	reportRaw, err := readPinned("/report.json", *reportSHA, 8<<20)
	if err != nil {
		return err
	}
	pgRaw, err := readPinned("/pg.json", *pgSHA, 8<<20)
	if err != nil {
		return err
	}
	diagnosticRaw, err := readPinned("/diagnostics.json", *diagnosticSHA, 8<<20)
	if err != nil {
		return err
	}
	corpusRaw, err := os.ReadFile("/src/" + acceptance.CloudV4CorpusPath)
	if err != nil {
		return errors.New("frozen corpus unavailable")
	}
	c, err := acceptance.LoadCloudV4Corpus(strings.NewReader(string(corpusRaw)))
	if err != nil {
		return errors.New("frozen corpus denied")
	}
	keyRaw, err := acceptance.ReadPrivateFile("/metadata/public-key.hex", 1024)
	if err != nil {
		return err
	}
	approvalRaw, err := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	if err != nil {
		return err
	}
	approval, key, err := acceptance.DecodeCloudV4GoldenArtifacts(c, keyRaw, approvalRaw)
	if err != nil {
		return errors.New("signed corpus denied")
	}
	expected := acceptance.CloudV4WindowExpectation{CodeCommit: *code, SourceSHA256: *source, HardwareSHA256: *hardware, HistorySHA256: acceptance.CloudV4HistorySHA256}
	audit, err := acceptance.AuditCloudV4Window(c, approval, key, reportRaw, expected)
	if err != nil {
		return errors.New("owning window audit denied")
	}
	correlation, err := acceptance.CorrelateCloudV4PGWitness(reportRaw, pgRaw)
	if err != nil || !correlation.Matched {
		return errors.New("owning SQL correlation denied")
	}
	var w acceptance.CloudV4PGWitness
	if acceptance.DecodeStrict(pgRaw, &w, "version", "window", "attempts") != nil {
		return errors.New("owning SQL schema denied")
	}
	var diagnostics struct {
		Version  string       `json:"version"`
		Attempts []diagnostic `json:"attempts"`
	}
	if acceptance.DecodeStrict(diagnosticRaw, &diagnostics, "version", "attempts") != nil || diagnostics.Version != "butler-cloud-v4-pg-diagnostics-v1" || len(diagnostics.Attempts) != len(w.Attempts) || len(audit.Attempts) != len(w.Attempts) {
		return errors.New("diagnostic inventory denied")
	}
	var e struct {
		Quality []acceptance.CloudAttempt     `json:"quality_attempts"`
		Single  acceptance.CloudComparisonRun `json:"single"`
		Multi   acceptance.CloudComparisonRun `json:"multi"`
	}
	if json.Unmarshal(reportRaw, &e) != nil {
		return errors.New("validated collector projection denied")
	}
	attempts := map[string]acceptance.CloudAttempt{}
	for _, a := range e.Quality {
		attempts[a.ContextID] = a
	}
	for _, arm := range []acceptance.CloudComparisonRun{e.Single, e.Multi} {
		for _, task := range arm.Tasks {
			for _, a := range task.Attempts {
				attempts[a.ContextID] = a
			}
		}
	}
	inputs, err := acceptance.ExportCloudV4Inputs(c, "")
	if err != nil {
		return errors.New("original input export denied")
	}
	goldens := map[string]acceptance.Example{}
	index := 0
	for _, original := range c.Cases {
		if original.NoModel {
			continue
		}
		if index >= len(inputs.Cases) {
			return errors.New("original selection drift")
		}
		goldens[inputs.Cases[index].ID] = acceptance.Example{Kind: original.Kind, Documents: original.Documents, Golden: original.Golden}
		index++
	}
	rawCount, witnessCount := 0, 0
	for i, d := range diagnostics.Attempts {
		p := w.Attempts[i]
		a, ok := attempts[d.Context]
		safe := audit.Attempts[i]
		if !ok || d.Ordinal != 269+i || d.Ordinal != p.Ordinal || d.Context != p.Context || d.Context != safe.Context || d.Outcome != p.Outcome || !reflect.DeepEqual(d.Input, p.InputTokens) || !reflect.DeepEqual(d.Output, p.OutputTokens) || !reflect.DeepEqual(d.Total, p.TotalTokens) || !reflect.DeepEqual(d.Version, a.ProviderModelVersion) {
			return errors.New("exact diagnostic SQL measurement mismatch")
		}
		if !d.Retained {
			if d.Outcome == "completed" {
				return errors.New("completed original bytes unavailable")
			}
			continue
		}
		if d.Bytes == nil || d.Hash == nil || !d.RetentionOK {
			return errors.New("private retained facts incomplete")
		}
		raw, err := readRaw(filepath.Join("/raw", fmt.Sprintf("final-%03d.raw", d.Ordinal)))
		if err != nil {
			return err
		}
		if len(raw) != *d.Bytes || hash(raw) != *d.Hash || encodingReason(raw) != d.Reason {
			return errors.New("private original byte/category mismatch")
		}
		rawCount++
		if d.Outcome == "completed" {
			if !d.Clean || d.Finals != 1 || d.Terminals != 1 || d.Finish == nil || *d.Finish != "STOP" || d.Version == nil || *d.Version != acceptance.CloudV4ExpectedModelVersion || d.Reason != "json_object" || d.Code != "" || d.Stage != "completed" {
				return errors.New("completed native state contradicted")
			}
			originalHash, err := acceptance.CloudV4OutputDigest(raw)
			if err != nil {
				return errors.New("completed raw encoding denied")
			}
			reportHash, err := acceptance.CloudV4OutputDigest(a.Output)
			if err != nil || originalHash != reportHash || p.OutputHash == nil || *p.OutputHash != originalHash {
				return errors.New("completed original/report/SQL output mismatch")
			}
			original, exists := goldens[a.CaseID]
			if !exists {
				return errors.New("foreign original witness")
			}
			check := acceptance.VerifyCloudV4Response(original, raw)
			if check.Exact {
				witnessCount++
			}
			if a.Role == acceptance.CloudRoleSynthesis {
				var children []string
				for _, role := range []string{acceptance.CloudRoleChildA, acceptance.CloudRoleChildB} {
					found := false
					for _, child := range attempts {
						if child.CaseID == a.CaseID && child.Role == role && child.Outcome == "completed" {
							children = append(children, string(child.Output))
							found = true
							break
						}
					}
					if !found {
						return errors.New("synthesis child provenance unavailable")
					}
				}
				var selected acceptance.CloudInputCase
				for _, candidate := range inputs.Cases {
					if candidate.ID == a.CaseID {
						selected = candidate
						break
					}
				}
				input, err := acceptance.MakeCloudV4ModelInput(selected, a.Role, children)
				if err != nil || acceptance.CloudV4ModelInputDigest(input) != a.InputSHA256 {
					return errors.New("synthesis causal input mismatch")
				}
			}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status        string                          `json:"status"`
		ReportSHA     string                          `json:"report_sha256"`
		PGSHA         string                          `json:"pg_sha256"`
		DiagnosticSHA string                          `json:"diagnostics_sha256"`
		Rows          int                             `json:"exact_diagnostic_rows"`
		RawCount      int                             `json:"exact_private_originals"`
		WitnessCount  int                             `json:"correct_original_witnesses"`
		Audit         acceptance.CloudV4WindowAudit   `json:"window_audit"`
		Correlation   acceptance.CloudV4PGCorrelation `json:"pg_correlation"`
		NativeCalls   int                             `json:"auditor_native_calls"`
		IssueReady    bool                            `json:"issue33_ready"`
	}{"V4_PRIVATE_FINAL_SQL_CORRELATION_MATCH", *reportSHA, *pgSHA, *diagnosticSHA, len(w.Attempts), rawCount, witnessCount, audit, correlation, 0, false})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
