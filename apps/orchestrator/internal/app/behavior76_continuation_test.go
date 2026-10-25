package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const behavior76CEntryLimit = 192
const behavior76OriginalDeadline = "2026-10-25T15:03:02.625452Z"
const behavior76BWindow = "issue76-local-20261025T110302"
const behavior76BManifestSHA = "f0b9f2f080910633ebf7cc15500974330183fe13dee99e00ec6bc8630342708c"
const behavior76BIndexSHA = "909109d041b54d817fb71a012fdbc201755e87bea7cd48a5945597afc3b39088"

// C may collect only the never-started suffix. It cannot recreate a failed B
// fixture, obtain a replacement answer, extend the original wall deadline, or
// change the model/candidate/controls/authority/recipient contracts.
func behavior76CheckPrefix(index behavior76Index, episodes []behavior76Episode, arms []string, count int) error {
	if count%2 != 0 || len(index.Episodes) != count || len(arms) != 2 || len(episodes)*2 < count {
		return errors.New("invalid immutable predecessor prefix")
	}
	for i, item := range index.Episodes {
		n := i / 2
		wantArm := arms[(n%2+i%2)%2]
		if item.EpisodeID != episodes[n].ID || item.MaskedArmID != wantArm || len(item.Steps) != len(episodes[n].Steps) {
			return errors.New("predecessor is not the exact completed original prefix")
		}
		for j, step := range item.Steps {
			if step.StepID != episodes[n].Steps[j].ID {
				return errors.New("a begun predecessor step cannot be omitted or replayed")
			}
		}
	}
	return nil
}

func behavior76CheckRawFiles(root, manifestPath, expectedHash string, expectedCount int) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil || behavior76Hash(raw) != expectedHash {
		return errors.New("original predecessor file manifest changed")
	}
	var files []struct {
		Path  string `json:"path"`
		SHA   string `json:"sha256"`
		Bytes int64  `json:"bytes"`
	}
	if json.Unmarshal(raw, &files) != nil || len(files) != expectedCount {
		return errors.New("original predecessor inventory unavailable")
	}
	actualCount := 0
	if err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("predecessor evidence cannot contain symlinks")
		}
		if !entry.IsDir() {
			actualCount++
		}
		return nil
	}); err != nil || actualCount != expectedCount {
		return errors.New("original predecessor file set changed")
	}
	seen := map[string]bool{}
	for _, file := range files {
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		if file.Path == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(file.Path, "\\") || seen[clean] {
			return errors.New("predecessor inventory contains an unsafe or duplicate path")
		}
		seen[clean] = true
		body, e := os.ReadFile(filepath.Join(root, clean))
		if e != nil || int64(len(body)) != file.Bytes || behavior76Hash(body) != file.SHA {
			return errors.New("original predecessor evidence bytes changed")
		}
	}
	return nil
}

func TestBehavior76ContinuationBoundary(t *testing.T) {
	episodes := []behavior76Episode{{ID: "public-a", Steps: []behavior76Step{{ID: "first"}, {ID: "second"}}}, {ID: "public-b", Steps: []behavior76Step{{ID: "first"}, {ID: "second"}}}, {ID: "never-started", Steps: []behavior76Step{{ID: "first"}, {ID: "second"}}}}
	index := behavior76Index{Episodes: []behavior76IndexEpisode{
		{EpisodeID: "public-a", MaskedArmID: "m1", Steps: []behavior76IndexStep{{StepID: "first"}, {StepID: "second"}}},
		{EpisodeID: "public-a", MaskedArmID: "m2", Steps: []behavior76IndexStep{{StepID: "first"}, {StepID: "second"}}},
		{EpisodeID: "public-b", MaskedArmID: "m2", Steps: []behavior76IndexStep{{StepID: "first"}, {StepID: "second"}}},
		{EpisodeID: "public-b", MaskedArmID: "m1", Steps: []behavior76IndexStep{{StepID: "first"}, {StepID: "second"}}},
	}}
	if err := behavior76CheckPrefix(index, episodes, []string{"m1", "m2"}, 4); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing-step", "wrong-arm", "wrong-episode", "extra-slot"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(index)
			var bad behavior76Index
			_ = json.Unmarshal(raw, &bad)
			switch kind {
			case "missing-step":
				bad.Episodes[3].Steps = bad.Episodes[3].Steps[:1]
			case "wrong-arm":
				bad.Episodes[3].MaskedArmID = "m2"
			case "wrong-episode":
				bad.Episodes[3].EpisodeID = "never-started"
			case "extra-slot":
				bad.Episodes = append(bad.Episodes, bad.Episodes[0])
			}
			if behavior76CheckPrefix(bad, episodes, []string{"m1", "m2"}, 4) == nil {
				t.Fatal("invalid continuation accepted")
			}
		})
	}
	root := t.TempDir()
	body := []byte("original public evidence\n")
	path := filepath.Join(root, "original.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]map[string]any{{"path": "original.json", "sha256": behavior76Hash(body), "bytes": len(body)}})
	manifest := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := behavior76CheckRawFiles(root, manifest, behavior76Hash(raw), 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if behavior76CheckRawFiles(root, manifest, behavior76Hash(raw), 1) == nil {
		t.Fatal("changed predecessor bytes accepted")
	}
}
