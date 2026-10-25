// Package maintenance owns independent release admission. A source inventory or
// passing author test is evidence data, never deployment permission.
package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrInventory = errors.New("maintenance source inventory rejected")

type Class string

const (
	Skill       Class = "skill"
	Tool        Class = "tool"
	Model       Class = "model_configuration"
	Application Class = "application"
	Kernel      Class = "protected_kernel"
)

// SourceFile describes measured bytes, not a host path or an author's label.
// Mode retains executable, link and submodule changes in the measured diff.
type SourceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Mode   string `json:"mode"`
}

// Boundary belongs to a fixed operator profile. It cannot unprotect core paths.
// Prefix must name an entire directory, including its trailing slash.
type Boundary struct {
	Prefix string `json:"prefix"`
	Class  Class  `json:"class"`
}

type SourceChange struct {
	Path   string      `json:"path"`
	Before *SourceFile `json:"before"`
	After  *SourceFile `json:"after"`
	Class  Class       `json:"class"`
}

type InventoryDiff struct {
	BeforeSHA256 string         `json:"before_sha256"`
	AfterSHA256  string         `json:"after_sha256"`
	Changes      []SourceChange `json:"changes"`
	Class        Class          `json:"class"`
}

func Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validSourcePath(value string) bool {
	if value == "" || len(value) > 512 || !utf8.ValidString(value) || path.Clean(value) != value || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\:") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
		for _, character := range part {
			if character < 32 || character == 127 {
				return false
			}
		}
	}
	return true
}

// This conservative list cannot be weakened by an ordinary operator boundary.
// Unknown paths and nonregular files are protected as well. A module is ordinary
// only when an independent operator profile explicitly confines it.
func protectedPath(value string) bool {
	value = strings.ToLower(value)
	// Existing Butler services/shared code own credentials, policy or durable
	// execution contracts. This first profile never automatically patches them.
	// Ordinary modules must be separately isolated, operator-pinned capsules;
	// an explicit boundary cannot reclassify the existing monolithic services.
	for _, prefix := range []string{".github/", "apps/", "cmd/", "deploy/", "internal/", "migrations/", "proto/", "scripts/"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return value == "go.mod" || value == "go.sum" || value == "dockerfile" || value == "agents.md" || strings.HasPrefix(path.Base(value), "dockerfile")
}

func classRank(class Class) int {
	switch class {
	case Skill:
		return 1
	case Tool:
		return 2
	case Model:
		return 3
	case Application:
		return 4
	case Kernel:
		return 5
	}
	return 0
}

func validateBoundaries(boundaries []Boundary) error {
	if len(boundaries) > 32 {
		return ErrInventory
	}
	for i, boundary := range boundaries {
		if !strings.HasSuffix(boundary.Prefix, "/") || !validSourcePath(strings.TrimSuffix(boundary.Prefix, "/")) || protectedPath(boundary.Prefix) || classRank(boundary.Class) == 0 || boundary.Class == Kernel {
			return ErrInventory
		}
		for _, previous := range boundaries[:i] {
			a, b := strings.ToLower(boundary.Prefix), strings.ToLower(previous.Prefix)
			if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
				return ErrInventory
			}
		}
	}
	return nil
}

func sourceInventory(files []SourceFile) (map[string]SourceFile, string, error) {
	if len(files) == 0 || len(files) > 100000 {
		return nil, "", ErrInventory
	}
	indexed := make(map[string]SourceFile, len(files))
	portable := make(map[string]bool, len(files))
	ordered := append([]SourceFile(nil), files...)
	var total int64
	for _, file := range files {
		key := strings.ToLower(file.Path)
		if !validSourcePath(file.Path) || !validDigest(file.SHA256) || file.Bytes < 0 || file.Bytes > 32<<20 || portable[key] || (file.Mode != "100644" && file.Mode != "100755" && file.Mode != "120000" && file.Mode != "160000") {
			return nil, "", ErrInventory
		}
		total += file.Bytes
		if total > 1<<30 {
			return nil, "", ErrInventory
		}
		portable[key] = true
		indexed[file.Path] = file
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	raw, err := json.Marshal(ordered)
	if err != nil {
		return nil, "", ErrInventory
	}
	return indexed, Digest(append([]byte("butler.maintenance.source-inventory.v1\n"), raw...)), nil
}

// CompareInventory computes every addition, deletion, mode and byte change.
// Inventories must come from independently measured builds. The controller must
// still verify their signed binding to both artifacts and the current profile.
func CompareInventory(before, after []SourceFile, boundaries []Boundary) (InventoryDiff, error) {
	if validateBoundaries(boundaries) != nil {
		return InventoryDiff{}, ErrInventory
	}
	oldFiles, oldHash, err := sourceInventory(before)
	if err != nil {
		return InventoryDiff{}, err
	}
	newFiles, newHash, err := sourceInventory(after)
	if err != nil {
		return InventoryDiff{}, err
	}
	paths := make(map[string]bool, len(oldFiles)+len(newFiles))
	for name := range oldFiles {
		paths[name] = true
	}
	for name := range newFiles {
		paths[name] = true
	}
	diff := InventoryDiff{BeforeSHA256: oldHash, AfterSHA256: newHash, Changes: []SourceChange{}}
	for name := range paths {
		oldFile, existed := oldFiles[name]
		newFile, exists := newFiles[name]
		if existed && exists && oldFile == newFile {
			continue
		}
		change := SourceChange{Path: name, Class: Kernel}
		if existed {
			copy := oldFile
			change.Before = &copy
		}
		if exists {
			copy := newFile
			change.After = &copy
		}
		regular := (!existed || oldFile.Mode == "100644" || oldFile.Mode == "100755") && (!exists || newFile.Mode == "100644" || newFile.Mode == "100755")
		if regular && !protectedPath(name) {
			for _, boundary := range boundaries {
				if strings.HasPrefix(name, boundary.Prefix) {
					change.Class = boundary.Class
				}
			}
		}
		if classRank(change.Class) > classRank(diff.Class) {
			diff.Class = change.Class
		}
		diff.Changes = append(diff.Changes, change)
	}
	sort.Slice(diff.Changes, func(i, j int) bool { return diff.Changes[i].Path < diff.Changes[j].Path })
	if len(diff.Changes) == 0 {
		return InventoryDiff{}, ErrInventory
	}
	return diff, nil
}
