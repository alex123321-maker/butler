package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
)

// Construct this from the recursively validated CLOSED latest leaf, never from
// a client-authored verdict or a sum of inclusive predecessor scopes. The native
// entry independently reloads that lineage before accepting its matching seal.
type experienceNativeLineage struct {
	Hash     string                 `json:"sha256"`
	Day      string                 `json:"day"`
	Deadline time.Time              `json:"deadline"`
	Carry    budget.Units           `json:"inclusive_carry"`
	Usage    temporalV5CurrentUsage `json:"inclusive_usage"`
}

type experienceNativeSeal struct {
	Format        string                  `json:"format"`
	Owner         string                  `json:"owner"`
	Created       time.Time               `json:"created_at"`
	Deadline      time.Time               `json:"deadline"`
	Lineage       experienceNativeLineage `json:"history"`
	Originals     string                  `json:"originals_sha256"`
	Controls      string                  `json:"controls_sha256"`
	Source        map[string]string       `json:"source_files"`
	Binaries      map[string]string       `json:"binaries"`
	Schedule      []experienceNativeSlot  `json:"schedule"`
	Policy        budget.Policy           `json:"control_policy"`
	SourcePolicy  budget.Policy           `json:"source_policy"`
	Environments  map[string]string       `json:"environments"`
	FGMillis      int64                   `json:"foreground_millis"`
	MaximumMillis int64                   `json:"whole_maximum_millis"`
}

func experienceNativeSHA(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func experienceNativeFileSHA(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512*1024*1024 {
		return "", errors.New("bounded regular frozen artifact required")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("frozen artifact unavailable")
	}
	defer f.Close()
	h := sha256.New()
	if n, copyErr := io.Copy(h, io.LimitReader(f, 512*1024*1024+1)); copyErr != nil || n != info.Size() {
		return "", errors.New("frozen artifact hash unavailable")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func experienceNativeSourcePins(root string) (map[string]string, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("absolute frozen source root required")
	}
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" || rel == ".cache" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(files) >= 20000 || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("bounded frozen source inventory required")
		}
		digest, err := experienceNativeFileSHA(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = digest
		return nil
	})
	if err != nil || files["go.mod"] == "" || len(files) == 0 {
		return nil, errors.New("complete regular frozen source inventory unavailable")
	}
	return files, nil
}

func experienceNativeBinaryPins(paths map[string]string) (map[string]string, error) {
	if len(paths) != 5 {
		return nil, errors.New("all five immutable compiled artifacts required")
	}
	pins := map[string]string{}
	for _, key := range []string{"recipient", "broker", "broker_control", "extension", "http"} {
		if !filepath.IsAbs(paths[key]) {
			return nil, errors.New("absolute immutable artifact path required")
		}
		if info, err := os.Lstat(paths[key]); err != nil || info.Size() == 0 || info.Mode().Perm()&0222 != 0 {
			return nil, errors.New("compiled artifact is empty, writable or unavailable")
		}
		digest, err := experienceNativeFileSHA(paths[key])
		if err != nil {
			return nil, err
		}
		pins[key] = digest
	}
	return pins, nil
}

func experienceNativeCarryPolicy(source bool, carry budget.Units) (budget.Policy, error) {
	p := experienceNativeBudgetPolicy()
	if source {
		p = experienceNativeSourcePolicy()
	}
	fresh := p.Day
	p.Day = consolidationAddUnits(fresh, carry)
	if !consolidationV2Within(carry, p.Day) || !consolidationV2Within(fresh, p.Day) || p.Validate() != nil {
		return budget.Policy{}, errors.New("finite nonnegative owner-day carry overflows or differs")
	}
	return p, nil
}

func validateExperienceNativeLineage(history experienceNativeLineage) error {
	u := history.Usage
	if !experienceNativeSHA(history.Hash) || u.NativeEntries < 0 || u.NativeKnownUsage < 0 || u.NativeUnknownUsage < 0 || u.ProductObserved < 0 || u.ProductNULL < 0 || u.NativeMissingTotal < 0 || u.NativeMissingTotal > u.NativeKnownUsage || u.KnownFullTokenLowerBound < 0 || u.NativeEntries != u.NativeKnownUsage+u.NativeUnknownUsage || u.ProductAttempts != u.ProductObserved+u.ProductNULL || u.ProductWithoutNativeEntry != u.ProductAttempts-u.NativeEntries || u.ProductWithoutNativeEntry < 0 || int64(u.ProductAttempts) != history.Carry.ModelTurns {
		return errors.New("complete current owner-day usage and charge metadata required")
	}
	_, err := experienceNativeCarryPolicy(false, history.Carry)
	return err
}

func prepareExperienceNativeSeal(now time.Time, sourceRoot string, corpus experienceNativeCorpus, history experienceNativeLineage, binaries map[string]string) (experienceNativeSeal, error) {
	var seal experienceNativeSeal
	if err := validateExperienceNativeLineage(history); err != nil {
		return seal, fmt.Errorf("complete inclusive lineage required: %w", err)
	}
	if !experienceNativeSHA(corpus.OriginalsSHA256) || !experienceNativeSHA(corpus.ControlsSHA256) {
		return seal, errors.New("exact mechanical corpus hashes required")
	}
	if err := validateExperienceNativeMechanicalInputs(corpus); err != nil {
		return seal, fmt.Errorf("fixed mechanical corpus rejected: %w", err)
	}
	deadline, err := experienceNativeWholeDeadline(now, history.Day, history.Deadline)
	if err != nil {
		return seal, err
	}
	files, err := experienceNativeSourcePins(sourceRoot)
	if err != nil {
		return seal, err
	}
	pins, err := experienceNativeBinaryPins(binaries)
	if err != nil {
		return seal, err
	}
	policy, err := experienceNativeCarryPolicy(false, history.Carry)
	if err != nil {
		return seal, err
	}
	sourcePolicy, err := experienceNativeCarryPolicy(true, history.Carry)
	if err != nil {
		return seal, err
	}
	environments := map[string]string{}
	for _, profile := range []struct {
		name, version, ready string
		outage               bool
	}{{"project-v1-normal", "v1", "true", false}, {"project-v1-receipt-outage", "v1", "true", true}, {"project-v2-normal", "v2", "verified", false}} {
		configuration := experienceEnvironment(experienceRecipientConfig{Version: profile.version, Ready: profile.ready, Outage: profile.outage})
		environments[profile.name] = experienceDigest(strings.Join([]string{configuration, pins["recipient"], pins["broker"], pins["extension"], pins["http"]}, "\x00"))
	}
	return experienceNativeSeal{Format: "butler.experience.native-preseal.v1", Owner: experienceNativeOwner, Created: now.UTC(), Deadline: deadline, Lineage: history, Originals: corpus.OriginalsSHA256, Controls: corpus.ControlsSHA256, Source: files, Binaries: pins, Schedule: experienceNativeSchedule(corpus), Policy: policy, SourcePolicy: sourcePolicy, Environments: environments, FGMillis: experienceNativeForegroundLimit.Milliseconds(), MaximumMillis: experienceNativeWholeMaximum.Milliseconds()}, nil
}

func checkExperienceNativeSeal(raw []byte, expectedSHA string, now time.Time, root string, corpus experienceNativeCorpus, history experienceNativeLineage, binaries map[string]string) (experienceNativeSeal, error) {
	var actual experienceNativeSeal
	if len(raw) > 8*1024*1024 || !experienceNativeSHA(expectedSHA) || experienceDigest(string(raw)) != expectedSHA {
		return actual, errors.New("exact immutable native preseal required")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&actual) != nil || d.Decode(new(any)) != io.EOF || actual.Created.After(now) || !actual.Deadline.After(now) || history.Day != now.UTC().Format("2006-01-02") {
		return actual, errors.New("current finite native preseal required")
	}
	expected, err := prepareExperienceNativeSeal(actual.Created, root, corpus, history, binaries)
	if err != nil || !reflect.DeepEqual(expected, actual) {
		return actual, errors.New("source, compiled artifacts, policies, history or schedule changed after seal")
	}
	return actual, nil
}

// This filesystem preflight runs offline, before the auth lock/provider snapshot.
// Existing permissions are not silently broadened or repaired.
func experienceNativePrivateParent(path string) error {
	if runtime.GOOS != "linux" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("absolute private native parent required")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("private native parent could not be prepared offline")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("private native parent must be an existing restricted directory")
	}
	return nil
}

func writeExperienceNativeSeal(parent, label string, seal experienceNativeSeal) (string, error) {
	if experienceNativePrivateParent(parent) != nil || len(label) < 1 || len(label) > 80 || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return "", errors.New("bounded fresh preseal identity and private parent required")
	}
	path := filepath.Join(parent, label)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", errors.New("native preseal identity was used or unavailable")
	}
	p, err := newExperienceNativeProvider(path, seal.Schedule, seal.Owner)
	if err != nil {
		return "", err
	}
	if err = p.writeNew("preseal.json", seal); err != nil {
		return "", err
	}
	return experienceNativeFileSHA(filepath.Join(path, "preseal.json"))
}

func TestExperienceNativePrivateParentPreflightPrecedesAuth(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux private-directory permission semantics required")
	}
	parent := filepath.Join(t.TempDir(), "missing-base", "private")
	if err := experienceNativePrivateParent(parent); err != nil {
		t.Fatal("missing parent was not prepared offline", err)
	}
	if err := experienceNativePrivateParent(parent + string(filepath.Separator) + ".." + string(filepath.Separator) + "relative"); err == nil {
		t.Fatal("noncanonical private parent accepted")
	}
	file := filepath.Join(parent, "ordinary-file")
	if os.WriteFile(file, []byte("controlled ordinary file"), 0600) != nil || experienceNativePrivateParent(file) == nil {
		t.Fatal("file masqueraded as private parent")
	}
}

func TestExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator(t *testing.T) {
	root, artifacts := t.TempDir(), t.TempDir()
	if os.WriteFile(filepath.Join(root, "go.mod"), []byte("module controlled-fixture\n"), 0600) != nil {
		t.Fatal("controlled source unavailable")
	}
	binaries := map[string]string{}
	for _, key := range []string{"recipient", "broker", "broker_control", "extension", "http"} {
		path := filepath.Join(artifacts, key)
		if os.WriteFile(path, []byte("controlled-compiled-"+key), 0500) != nil {
			t.Fatal("controlled artifact unavailable")
		}
		binaries[key] = path
	}
	now := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	history := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: "2026-10-18", Deadline: now.Add(16 * time.Hour), Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344, ActiveMillis: 900000, ProviderInputBytes: 3 * 8388608}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	c := experienceWindowFixture()
	c.Originals.CommonProfile = json.RawMessage(`{"answer_language":"ru","no_other_preferences":true}`)
	c.OriginalsSHA256, c.ControlsSHA256 = strings.Repeat("b", 64), strings.Repeat("c", 64)
	for group, items := range [][]experienceNativeTask{c.Originals.Tasks, c.Controls.Cases} {
		for i := range items {
			prefix := "original"
			if group == 1 {
				prefix = "control"
			}
			items[i].ID = fmt.Sprintf("%s-%02d", prefix, i+1)
			items[i].EnvironmentProfile, items[i].PermissionProfile, items[i].Reset = "project-v1-normal", "prepare_and_verify_exact_resource", "empty_patch_store_original_readme"
		}
	}
	seal, err := prepareExperienceNativeSeal(now, root, c, history, binaries)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(seal)
	if _, err = checkExperienceNativeSeal(raw, experienceDigest(string(raw)), now.Add(time.Second), root, c, history, binaries); err != nil || len(seal.Schedule) != 57 || seal.Policy.Day.ModelTurns != 643 || seal.SourcePolicy.Task.ModelTurns != 13 || seal.Lineage.Usage.NativeEntries != 0 || seal.Lineage.Usage.ProductNULL != 3 {
		t.Fatal("preseal lost current charges or changed the full denominator", err)
	}
	for name, mutate := range map[string]func(*experienceNativeSeal){
		"history_refund":    func(s *experienceNativeSeal) { s.Lineage.Carry.ModelTurns-- },
		"hidden_native":     func(s *experienceNativeSeal) { s.Lineage.Usage.NativeEntries = 3 },
		"extra_slot":        func(s *experienceNativeSeal) { s.Schedule = append(s.Schedule, s.Schedule[0]) },
		"changed_budget":    func(s *experienceNativeSeal) { s.Policy.Task.ModelTurns++ },
		"changed_recipient": func(s *experienceNativeSeal) { s.Binaries["recipient"] = strings.Repeat("d", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			var changed experienceNativeSeal
			_ = json.Unmarshal(raw, &changed)
			mutate(&changed)
			modified, _ := json.Marshal(changed)
			if _, err := checkExperienceNativeSeal(modified, experienceDigest(string(modified)), now.Add(time.Second), root, c, history, binaries); err == nil {
				t.Fatal("changed preseal was accepted")
			}
		})
	}
	if os.WriteFile(filepath.Join(root, "added.go"), []byte("package controlled\n"), 0600) != nil {
		t.Fatal("controlled source mutation unavailable")
	}
	if _, err = checkExperienceNativeSeal(raw, experienceDigest(string(raw)), now.Add(time.Second), root, c, history, binaries); err == nil {
		t.Fatal("additional source file escaped the exact inventory")
	}
	if os.Remove(filepath.Join(root, "added.go")) != nil || os.Chmod(binaries["recipient"], 0700) != nil || os.WriteFile(binaries["recipient"], []byte("changed-controlled-compiled-recipient"), 0700) != nil || os.Chmod(binaries["recipient"], 0500) != nil {
		t.Fatal("controlled compiled-artifact mutation unavailable")
	}
	if _, err = checkExperienceNativeSeal(raw, experienceDigest(string(raw)), now.Add(time.Second), root, c, history, binaries); err == nil {
		t.Fatal("changed compiled recipient escaped the sealed binary identity")
	}
	if _, err = checkExperienceNativeSeal(raw, experienceDigest(string(raw)), history.Deadline, root, c, history, binaries); err == nil {
		t.Fatal("expired owner-day window was reopened")
	}
}
