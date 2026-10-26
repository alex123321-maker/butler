package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
)

// ObservationChallenge contains no target/configuration/key selection. A stale
// epoch can produce historical evidence but cannot bypass Store currentness.
type ObservationChallenge struct {
	ProfileSHA256 string `json:"profile_sha256"`
	Epoch         int64  `json:"epoch"`
	Nonce         string `json:"nonce"`
}

type SignedNodeObservation struct {
	Raw       []byte `json:"raw"`
	Signature []byte `json:"signature"`
}

// NodeObserver owns exactly one real read-only inspector and one private node
// signing key. It cannot sign caller-supplied identity, effect settlement or an
// arbitrary payload, launch a process, deploy code or clear UNKNOWN.
type NodeObserver struct {
	inspector *DockerInspector
	profile   Profile
	digest    string
	key       ed25519.PrivateKey
}

func NewNodeObserver(socket, container string, profileRaw []byte, key ed25519.PrivateKey) (*NodeObserver, error) {
	profile, err := ParseProfile(profileRaw)
	if err != nil || len(key) != ed25519.PrivateKeySize || !bytes.Equal(key.Public().(ed25519.PublicKey), profile.NodePublicKey) {
		return nil, ErrState
	}
	inspector, err := NewDockerInspector(socket, container)
	if err != nil {
		return nil, ErrState
	}
	return &NodeObserver{inspector: inspector, profile: profile, digest: Digest(profileRaw), key: bytes.Clone(key)}, nil
}

func (node *NodeObserver) Observe(ctx context.Context, challenge ObservationChallenge) (SignedNodeObservation, error) {
	if node == nil || node.inspector == nil || len(node.key) != ed25519.PrivateKeySize || challenge.ProfileSHA256 != node.digest || challenge.Epoch < 1 || !validDigest(challenge.Nonce) {
		return SignedNodeObservation{}, ErrState
	}
	physical, err := node.inspector.Inspect(ctx)
	if err != nil {
		return SignedNodeObservation{}, ErrState
	}
	observation := NodeObservation{
		Version: "butler.maintenance.node-observation.v1", OwnerID: node.profile.OwnerID,
		TargetID: node.profile.TargetID, ProfileSHA256: node.digest, Epoch: challenge.Epoch,
		Nonce: challenge.Nonce, State: physical.State, ArtifactSHA256: physical.ArtifactSHA256,
		RuntimeSHA256: physical.RuntimeSHA256, ObservedAt: physical.MeasuredAt,
		// An inspection alone never establishes that an earlier effect settled.
		LastAttemptID: "", EffectSettled: false,
	}
	raw, signature, err := SignNodeObservation(observation, node.key)
	if err != nil {
		return SignedNodeObservation{}, ErrState
	}
	return SignedNodeObservation{Raw: raw, Signature: signature}, nil
}

func ParseObservationChallenge(raw []byte) (ObservationChallenge, error) {
	var challenge ObservationChallenge
	if len(raw) == 0 || len(raw) > 4096 || json.Unmarshal(raw, &challenge) != nil {
		return ObservationChallenge{}, ErrState
	}
	canonical, err := json.Marshal(challenge)
	if err != nil || !bytes.Equal(raw, canonical) || !validDigest(challenge.ProfileSHA256) || !validDigest(challenge.Nonce) || challenge.Epoch < 1 {
		return ObservationChallenge{}, ErrState
	}
	return challenge, nil
}
