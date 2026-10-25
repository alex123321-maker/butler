package maintenance

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrPermission = errors.New("exact owner maintenance permission required")

const permissionDomain = "butler.maintenance.owner-permission.v1\n"

// OwnerPermission is issued only after the separate controller authenticates an
// explicit owner operation. A review, model proposal or ordinary tool grant is
// insufficient. Epoch/nonce/request digest prevent reuse after a new owner stop
// or accidental reissuance from a repeated owner request.
type OwnerPermission struct {
	Version            string    `json:"version"`
	Kind               string    `json:"kind"`
	OwnerID            string    `json:"owner_id"`
	TargetID           string    `json:"target_id"`
	ProfileSHA256      string    `json:"profile_sha256"`
	Epoch              int64     `json:"epoch"`
	Nonce              string    `json:"nonce"`
	RequestSHA256      string    `json:"request_sha256"`
	MaxClaims          int64     `json:"max_claims"`
	AllowedClasses     []Class   `json:"allowed_classes"`
	StageID            string    `json:"stage_id"`
	ReviewSHA256       string    `json:"review_sha256"`
	BaseArtifactSHA256 string    `json:"base_artifact_sha256"`
	ArtifactSHA256     string    `json:"artifact_sha256"`
	AllowWhileStopped  bool      `json:"allow_while_stopped"`
	IssuedAt           time.Time `json:"issued_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

// SignOwnerPermission must not be exposed as a sign-arbitrary-bytes endpoint.
// Its private key belongs exclusively to the authenticated maintenance controller.
func SignOwnerPermission(permission OwnerPermission, key ed25519.PrivateKey) ([]byte, []byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, nil, ErrPermission
	}
	permission.IssuedAt = permission.IssuedAt.UTC().Truncate(time.Microsecond)
	permission.ExpiresAt = permission.ExpiresAt.UTC().Truncate(time.Microsecond)
	raw, err := json.Marshal(permission)
	if err != nil {
		return nil, nil, ErrPermission
	}
	return raw, ed25519.Sign(key, append([]byte(permissionDomain), raw...)), nil
}

// InspectOwnerPermission authenticates bounded historical permission too; expiry,
// revocation, consumption and current epoch are rechecked before an action claim.
func InspectOwnerPermission(raw, signature []byte, profile Profile, profileDigest string, now time.Time) (OwnerPermission, error) {
	profileRaw, err := json.Marshal(profile)
	if err != nil || Digest(profileRaw) != profileDigest || len(raw) == 0 || len(raw) > 65536 || len(profile.ControllerPublicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(profile.ControllerPublicKey), append([]byte(permissionDomain), raw...), signature) {
		return OwnerPermission{}, ErrPermission
	}
	var permission OwnerPermission
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&permission) != nil {
		return OwnerPermission{}, ErrPermission
	}
	canonical, err := json.Marshal(permission)
	if err != nil || !bytes.Equal(raw, canonical) || permission.Version != "butler.maintenance.owner-permission.v1" || permission.OwnerID != profile.OwnerID || permission.TargetID != profile.TargetID || permission.ProfileSHA256 != profileDigest || permission.Epoch <= 0 || !validDigest(permission.Nonce) || !validDigest(permission.RequestSHA256) || permission.MaxClaims < 1 || permission.MaxClaims > 10 || permission.IssuedAt.IsZero() || permission.IssuedAt.After(now) || !permission.ExpiresAt.After(permission.IssuedAt) || permission.ExpiresAt.Sub(permission.IssuedAt) > 24*time.Hour {
		return OwnerPermission{}, ErrPermission
	}
	if permission.IssuedAt.Nanosecond()%1000 != 0 || permission.ExpiresAt.Nanosecond()%1000 != 0 {
		return OwnerPermission{}, ErrPermission
	}
	switch permission.Kind {
	case "ordinary_mandate":
		if len(permission.AllowedClasses) == 0 || len(permission.AllowedClasses) > 4 || permission.StageID != "" || permission.ReviewSHA256 != "" || permission.BaseArtifactSHA256 != "" || permission.ArtifactSHA256 != "" || permission.AllowWhileStopped {
			return OwnerPermission{}, ErrPermission
		}
		seen := map[Class]bool{}
		for _, class := range permission.AllowedClasses {
			if classRank(class) == 0 || class == Kernel || seen[class] {
				return OwnerPermission{}, ErrPermission
			}
			seen[class] = true
		}
	case "exact_protected", "exact_rollback":
		if permission.MaxClaims != 1 || len(permission.AllowedClasses) != 0 || !strings.HasPrefix(permission.StageID, "mstage_") || !validDigest(strings.TrimPrefix(permission.StageID, "mstage_")) || !validDigest(permission.ReviewSHA256) || !validDigest(permission.BaseArtifactSHA256) || !validDigest(permission.ArtifactSHA256) || permission.BaseArtifactSHA256 == permission.ArtifactSHA256 || (permission.Kind == "exact_protected" && permission.AllowWhileStopped) {
			return OwnerPermission{}, ErrPermission
		}
	default:
		return OwnerPermission{}, ErrPermission
	}
	return permission, nil
}

// checkPermissionScope checks an authenticated permission against the exact
// current target/binding. It does not reserve budget, consume permission, verify
// a report, prove compatibility or dispatch. All those gates remain mandatory.
func checkPermissionScope(permission OwnerPermission, binding Binding, reviewDigest, purpose string, target Target, now time.Time) error {
	if !binding.valid() || !validDigest(reviewDigest) || permission.OwnerID != target.OwnerID || permission.TargetID != target.TargetID || permission.ProfileSHA256 != target.ProfileSHA256 || binding.ProfileSHA256 != target.ProfileSHA256 || permission.Epoch != target.Epoch || !permission.ExpiresAt.After(now) || permission.IssuedAt.After(now) || target.UnresolvedAttempt != "" || target.ObservationSHA256 == "" || target.ObservedAt == nil || target.ObservedAt.After(now) || now.Sub(*target.ObservedAt) > 30*time.Second {
		return ErrPermission
	}
	bindingRaw, err := json.Marshal(binding)
	if err != nil {
		return ErrPermission
	}
	stageID := "mstage_" + Digest(bindingRaw)
	if purpose == "promote" {
		if target.Stopped || target.ArtifactSHA256 != binding.BaseArtifactSHA256 || target.InventorySHA256 != binding.BeforeInventorySHA256 {
			return ErrPermission
		}
		if permission.Kind == "ordinary_mandate" && binding.Class != Kernel {
			for _, class := range permission.AllowedClasses {
				if class == binding.Class {
					return nil
				}
			}
		} else if permission.Kind == "exact_protected" && binding.Class == Kernel && permission.StageID == stageID && permission.ReviewSHA256 == reviewDigest && permission.BaseArtifactSHA256 == binding.BaseArtifactSHA256 && permission.ArtifactSHA256 == binding.CandidateSHA256 {
			return nil
		}
	} else if purpose == "rollback" {
		// A rollback is a separately measured reverse stage, not authority to
		// reuse the old forward report as evidence for the retained executable.
		if permission.Kind == "exact_rollback" && (!target.Stopped || permission.AllowWhileStopped) && target.ArtifactSHA256 == binding.BaseArtifactSHA256 && target.InventorySHA256 == binding.BeforeInventorySHA256 && permission.StageID == stageID && permission.ReviewSHA256 == reviewDigest && permission.BaseArtifactSHA256 == binding.BaseArtifactSHA256 && permission.ArtifactSHA256 == binding.CandidateSHA256 {
			return nil
		}
	}
	return ErrPermission
}

// VerifyPermissionForAction composes signature and scope checks. Its result is
// evidence for a controller/node transaction, not authority to skip live database
// revocation, one-use consumption, independent checks, compatibility or fencing.
func VerifyPermissionForAction(raw, signature []byte, profile Profile, profileDigest string, binding Binding, reviewDigest, purpose string, target Target, now time.Time) (OwnerPermission, error) {
	permission, err := InspectOwnerPermission(raw, signature, profile, profileDigest, now)
	if err != nil || checkPermissionScope(permission, binding, reviewDigest, purpose, target, now) != nil {
		return OwnerPermission{}, ErrPermission
	}
	return permission, nil
}
