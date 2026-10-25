package maintenance

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"time"
)

const observationDomain = "butler.maintenance.node-observation.v1\n"

// NodeObservation reports physical artifact/configuration identity only. It is
// not a functional check, source build attestation, owner decision or completion.
// Nonce/epoch bind the exact independent inspection request, never a model call.
type NodeObservation struct {
	Version        string    `json:"version"`
	OwnerID        string    `json:"owner_id"`
	TargetID       string    `json:"target_id"`
	ProfileSHA256  string    `json:"profile_sha256"`
	Epoch          int64     `json:"epoch"`
	Nonce          string    `json:"nonce"`
	State          string    `json:"state"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	RuntimeSHA256  string    `json:"runtime_sha256"`
	LastAttemptID  string    `json:"last_attempt_id"`
	ObservedAt     time.Time `json:"observed_at"`
}

// SignNodeObservation belongs only to the independent node process after its
// own fixed-target inspection. There is no author-accessible signing endpoint.
func SignNodeObservation(observation NodeObservation, key ed25519.PrivateKey) ([]byte, []byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, nil, ErrState
	}
	// PostgreSQL stores microsecond timestamps; use the same signed precision so
	// a round trip cannot make an older response appear newer by sub-microseconds.
	observation.ObservedAt = observation.ObservedAt.UTC().Truncate(time.Microsecond)
	raw, err := json.Marshal(observation)
	if err != nil {
		return nil, nil, ErrState
	}
	return raw, ed25519.Sign(key, append([]byte(observationDomain), raw...)), nil
}

// InspectNodeObservation authenticates historical observations too. The caller
// must independently enforce current profile/epoch/freshness before using them.
func InspectNodeObservation(raw, signature []byte, profile Profile, profileDigest, nonce string, epoch int64, now time.Time) (NodeObservation, error) {
	profileRaw, err := json.Marshal(profile)
	if err != nil || Digest(profileRaw) != profileDigest || len(raw) == 0 || len(raw) > 16384 || len(profile.NodePublicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !validDigest(nonce) || epoch <= 0 || !ed25519.Verify(ed25519.PublicKey(profile.NodePublicKey), append([]byte(observationDomain), raw...), signature) {
		return NodeObservation{}, ErrState
	}
	var observation NodeObservation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&observation) != nil {
		return NodeObservation{}, ErrState
	}
	canonical, err := json.Marshal(observation)
	if err != nil || !bytes.Equal(raw, canonical) || observation.Version != "butler.maintenance.node-observation.v1" || observation.OwnerID != profile.OwnerID || observation.TargetID != profile.TargetID || observation.ProfileSHA256 != profileDigest || observation.Nonce != nonce || observation.Epoch != epoch || observation.ObservedAt.IsZero() || observation.ObservedAt.After(now) || observation.ObservedAt.Nanosecond()%1000 != 0 {
		return NodeObservation{}, ErrState
	}
	if observation.State == "missing" {
		if observation.ArtifactSHA256 != "" || observation.RuntimeSHA256 != "" || observation.LastAttemptID != "" {
			return NodeObservation{}, ErrState
		}
	} else if (observation.State != "running" && observation.State != "stopped") || !validDigest(observation.ArtifactSHA256) || !validDigest(observation.RuntimeSHA256) {
		return NodeObservation{}, ErrState
	}
	if observation.LastAttemptID != "" && (len(observation.LastAttemptID) != len("mdeploy_")+64 || observation.LastAttemptID[:len("mdeploy_")] != "mdeploy_" || !validDigest(observation.LastAttemptID[len("mdeploy_"):])) {
		return NodeObservation{}, ErrState
	}
	return observation, nil
}
