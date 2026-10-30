package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/acceptance/utilitytrace"
)

func replayBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err == nil && len(b) > 32<<20 {
		err = errors.New("oversized trace artifact")
	}
	return b, err
}
func bytesSHA(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// The caller pins the preexisting collector manifest. Recomputed JSON seals are
// accepted only after the exact raw snapshot/binding hashes match that manifest.
func replayTraceRoot(root, manifestSHA, output string) error {
	manifest, err := replayBytes(filepath.Join(root, "public-report.json"))
	if err != nil {
		return err
	}
	if len(manifestSHA) != 64 || bytesSHA(manifest) != manifestSHA {
		return errors.New("original manifest mismatch")
	}
	var report struct {
		Source    string `json:"source"`
		Model     string `json:"model"`
		Scheduled int    `json:"scheduled"`
		SuiteSHA  string `json:"suite_sha256_bytes"`
		Rows      []struct {
			Sequence int               `json:"sequence"`
			Arm      string            `json:"arm"`
			Hashes   map[string]string `json:"artifact_hashes"`
		} `json:"rows"`
	}
	if err = json.Unmarshal(manifest, &report); err != nil {
		return err
	}
	if report.Scheduled != 36 || len(report.Rows) != 36 {
		return errors.New("complete workflow cohort required")
	}
	b, err := replayBytes(filepath.Join(root, "out", "plan", "suite.private.json"))
	if err != nil {
		return err
	}
	if bytesSHA(b) != report.SuiteSHA {
		return errors.New("frozen suite byte mismatch")
	}
	var suite acceptance.UtilitySuite
	if err = json.Unmarshal(b, &suite); err != nil {
		return err
	}
	if len(suite.Cases) != 12 || suite.Validate() != nil {
		return errors.New("invalid workflow suite")
	}
	results := []map[string]any{}
	seen := map[string]bool{}
	legacyTotals, pagedTotals := map[string]int{}, map[string]int{}
	for n, row := range report.Rows {
		if row.Sequence != n+1 || row.Arm != "A" && row.Arm != "B" && row.Arm != "C" {
			return errors.New("invalid original schedule")
		}
		dir := filepath.Join(root, "out", "actual", fmt.Sprintf("slot-%03d", row.Sequence))
		read := func(name string, dst any) error {
			raw, err := replayBytes(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			if row.Hashes[name] == "" || bytesSHA(raw) != row.Hashes[name] {
				return fmt.Errorf("original %s changed", name)
			}
			return json.Unmarshal(raw, dst)
		}
		var binding utilitytrace.Binding
		var snapshot utilitytrace.Snapshot
		var saved acceptance.UtilityResult
		var execution struct {
			ProjectionError string `json:"projection_error"`
		}
		for _, item := range []struct {
			name string
			dst  any
		}{
			{"binding.json", &binding}, {"snapshot.private.json", &snapshot}, {"score.json", &saved}, {"execution.json", &execution},
		} {
			if err = read(item.name, item.dst); err != nil {
				return err
			}
		}
		key := binding.CaseID + ":" + binding.Arm
		if seen[key] || binding.Arm != row.Arm {
			return errors.New("duplicate or mismatched case")
		}
		seen[key] = true
		seal := acceptance.MemoryDigest(snapshot)
		legacy, legacyErr := utilitytrace.Project(suite, binding, snapshot, seal)
		legacyScore := acceptance.EvaluateMemoryUtility(suite, legacy)
		if acceptance.MemoryDigest(legacyScore) != acceptance.MemoryDigest(saved) || fmt.Sprint(legacyErr) != execution.ProjectionError {
			return fmt.Errorf("legacy replay differs at slot %d", row.Sequence)
		}
		paged, pagedErr := utilitytrace.ProjectPagedReads(suite, binding, snapshot, seal)
		pagedScore := acceptance.EvaluateMemoryUtility(suite, paged)
		if legacyScore.TaskSuccess {
			legacyTotals[row.Arm]++
		}
		if pagedScore.TaskSuccess {
			pagedTotals[row.Arm]++
		}
		results = append(results, map[string]any{"sequence": row.Sequence, "case_id": binding.CaseID, "arm": row.Arm,
			"legacy": legacyScore, "paged": pagedScore, "legacy_projection_error": fmt.Sprint(legacyErr),
			"paged_projection_error": fmt.Sprint(pagedErr), "changed": acceptance.MemoryDigest(legacyScore) != acceptance.MemoryDigest(pagedScore)})
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	return writeNew(output, map[string]any{"policy": utilitytrace.PagedReadsVersion, "native_source": report.Source,
		"model": report.Model, "suite_sha256_bytes": report.SuiteSHA, "original_manifest_sha256": manifestSHA,
		"evaluator_binary_sha256": bytesSHA(binary), "scheduled": len(results), "native_requests": 0,
		"legacy_reproduced_exactly": true, "legacy_success": legacyTotals, "paged_success": pagedTotals, "rows": results})
}
