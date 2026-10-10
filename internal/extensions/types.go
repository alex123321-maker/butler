package extensions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

var (
	ErrUnavailable = errors.New("reviewed extension is unavailable")
	ErrConflict    = errors.New("extension version or configuration changed")
	ErrAccess      = errors.New("current extension permission is required")
)

// Profile is operator deployment configuration, never package/model data.
// Dependency pins record review provenance; they do not self-attest image integrity.
type Profile struct {
	Target     string     `json:"target"`
	Dependency Dependency `json:"dependency"`
}
type Record struct {
	ID             string                              `json:"extension_id"`
	OwnerID        string                              `json:"owner_id"`
	ManifestHash   string                              `json:"manifest_hash"`
	Generation     int64                               `json:"generation"`
	Status         string                              `json:"status"`
	Configuration  map[string]string                   `json:"configuration"`
	CredentialRefs map[string]actionauth.CredentialRef `json:"credential_refs"`
	AllowedTools   []string                            `json:"allowed_tools"`
	Health         string                              `json:"health"`
	HealthReason   string                              `json:"health_reason"`
	HealthyUntil   *time.Time                          `json:"healthy_until,omitempty"`
	Reviewed       bool                                `json:"reviewed"`
	Available      bool                                `json:"available"`
	Package        Package                             `json:"-"`
}
type TaskAccess struct {
	TaskID              string                     `json:"task_id"`
	TaskRevision        int64                      `json:"task_revision"`
	ConstraintsRevision int64                      `json:"constraints_revision"`
	ManifestHash        string                     `json:"manifest_hash"`
	Generation          int64                      `json:"generation"`
	Revision            int64                      `json:"revision"`
	Tools               []string                   `json:"tools"`
	Resources           []actionauth.ResourceScope `json:"resources"`
	CredentialRefs      []actionauth.CredentialRef `json:"credential_refs"`
	ExpiresAt           time.Time                  `json:"expires_at"`
	Revoked             bool                       `json:"revoked"`
}
type Probe struct {
	ID           string `json:"probe_id"`
	ExtensionID  string `json:"extension_id"`
	ManifestHash string `json:"manifest_hash"`
	Generation   int64  `json:"generation"`
	ConfigHash   string `json:"config_hash"`
	URL          string `json:"-"`
	State        string `json:"state"`
}

func hashJSON(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func has(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
