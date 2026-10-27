package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const local115NativeClosedFormat = "butler.experience.closed-local-owner-leaf.v1"
const local115NativeClosedPreseal = "dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099"
const local115NativeClosedPrevious = "579aa8c25bcfdba06b1be91f2cb613f2550ee685932c3919e2468bef13f11ec7"
const local115NativeClosedInventory = "dbc8d6baa126c1b59ad42dc07a6677d57badb6430da2613f361a28e42a6ec104"
const local115NativeClosedArchive = "0c949fb275d1f69f234b07b95e312f67692f9aa1c2e7ae3d84ba4aa6fe4c468d"
const local115NativeClosedHandback = "6ec6a4a366ed448ae460779ef7c5272ca518a91f73117259f863bf61a9febd19"

// Reuse the original full plan, quote-union, native/product correlation,
// deadline, denominator and durable journal checks. A local handback never
// satisfies the legacy lab-main handback contract or grants native authority.
func loadClosedLocal115NativeHistory(path, expected string) (experienceNativeLineage, error) {
	return loadClosedExperienceNativeHistoryProfile(path, expected, true)
}

func validateLocal115ClosedRawPopulation(root string, files map[string]string) error {
	raw, err := closedExperienceRead(root, "sealed-local-raw-inventory.json", local115NativeClosedInventory, 1024*1024)
	var original []struct {
		Path   string `json:"path"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	if err != nil || json.Unmarshal(raw, &original) != nil || len(original) != 157 || len(files) != 157 {
		return errors.New("original complete local raw inventory required")
	}
	seen := map[string]bool{}
	for _, entry := range original {
		name := "actual/" + entry.Path
		if seen[name] || files[name] != entry.SHA256 || entry.Bytes < 0 || entry.Bytes > 512*1024*1024 {
			return errors.New("local raw inventory identity differs")
		}
		seen[name] = true
		if _, err = closedExperienceRead(root, name, entry.SHA256, entry.Bytes); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || info.Size() != entry.Bytes {
			return errors.New("local raw inventory size differs")
		}
	}
	return nil
}

type local115ClosedHandback struct {
	Version      string `json:"version"`
	Plan         string `json:"plan_sha256"`
	Wait         bool   `json:"cmd_wait_returned"`
	Started      bool   `json:"collector_started"`
	Forced       *bool  `json:"forced_cleanup"`
	Snapshot     bool   `json:"fresh_local_encrypted_snapshot_saved"`
	Lab          *bool  `json:"lab_main_auth_claim"`
	Resolved     bool   `json:"local_auth_resolved"`
	BackendsGone bool   `json:"native_database_backends_gone"`
	Native       *int   `json:"native_exit_status"`
	CAS          bool   `json:"opaque_source_cas"`
	Raw          *bool  `json:"raw_material_printed"`
	Recovery     string `json:"recovery_source"`
	Qualified    *bool  `json:"release_qualified"`
	Closed       bool   `json:"source_windows_closed_checked"`
	Empty        bool   `json:"whole_cgroup_empty"`
}

func (h local115ClosedHandback) validate() error {
	if h.Version != "butler115-local-provider-handback.v1" || h.Plan != local115NativeClosedPreseal || !h.Wait || !h.Started || h.Forced == nil || *h.Forced || !h.Snapshot || h.Lab == nil || *h.Lab || !h.Resolved || !h.BackendsGone || h.Native == nil || *h.Native != 1 || !h.CAS || h.Raw == nil || *h.Raw || h.Recovery != "latest_durable_local_setting" || h.Qualified == nil || *h.Qualified || !h.Closed || !h.Empty {
		return errors.New("exact failed collector and separate local handback required")
	}
	return nil
}

func TestLocal115ClosedHandbackProfileBoundaries(t *testing.T) {
	root := "../../../../experiments/memory115/2026-10-26/closed-local-native-v1"
	raw, err := closedExperienceRead(root, "local-handback.json", local115NativeClosedHandback, 4096)
	if err != nil {
		t.Fatal(err)
	}
	var actual local115ClosedHandback
	if json.Unmarshal(raw, &actual) != nil || actual.validate() != nil {
		t.Fatal("published exact failed local handback was not recognized")
	}
	var legacy closedExperienceHandback
	if json.Unmarshal(raw, &legacy) != nil || legacy.validate() == nil {
		t.Fatal("local handback satisfied the distinct legacy lab-main contract")
	}
	for name, mutate := range map[string]func(*local115ClosedHandback){
		"changed_native_outcome":  func(h *local115ClosedHandback) { *h.Native = 0 },
		"forced_cleanup":          func(h *local115ClosedHandback) { *h.Forced = true },
		"claimed_lab_handback":    func(h *local115ClosedHandback) { *h.Lab = true },
		"omitted_false_claim":     func(h *local115ClosedHandback) { h.Lab = nil },
		"lost_backend_quiescence": func(h *local115ClosedHandback) { h.BackendsGone = false },
	} {
		t.Run(name, func(t *testing.T) {
			var changed local115ClosedHandback
			if json.Unmarshal(raw, &changed) != nil {
				t.Fatal("published handback unavailable")
			}
			mutate(&changed)
			if changed.validate() == nil {
				t.Fatal("altered local handback qualified")
			}
		})
	}
}

func TestLocal115ClosedNativeHistoryImport(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_CLOSED_LOCAL_IMPORT") != "exact-failed-local-cohort-only" {
		t.Skip("explicit immutable closed local history only")
	}
	history, err := loadClosedLocal115NativeHistory(os.Getenv("BUTLER_ISSUE115_CLOSED_LOCAL_MANIFEST"), os.Getenv("BUTLER_ISSUE115_CLOSED_LOCAL_SHA256"))
	if err != nil {
		t.Fatal(err)
	}
	u := history.Usage
	if u.NativeEntries != 232 || u.NativeKnownUsage != 231 || u.NativeUnknownUsage != 1 || u.ProductAttempts != 233 || u.ProductObserved != 231 || u.ProductNULL != 2 || u.ProductWithoutNativeEntry != 1 || u.KnownFullTokenLowerBound != 1738466 || history.Carry.ModelTurns != 233 {
		t.Fatal("original local inclusive population or uncertainty changed")
	}
	t.Log("CLOSED_LOCAL_HISTORY: original157 raw files and prior213 recursively retained;232native/231known/1unknown;233product/231observed/2NULL/1withoutNative;1738466known full tokens; noauth/PG/native or release qualification")
}
