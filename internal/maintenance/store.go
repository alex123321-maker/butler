package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrState = errors.New("maintenance state unavailable or stale")

// Store owns maintenance records only. Its operator mutation methods require
// the separate authenticated controller; they are not model/tool capabilities.
type Store struct{ Pool *pgxpool.Pool }

type Target struct {
	OwnerID           string     `json:"owner_id"`
	TargetID          string     `json:"target_id"`
	ProfileSHA256     string     `json:"profile_sha256"`
	Stopped           bool       `json:"stopped"`
	Epoch             int64      `json:"epoch"`
	NextFence         int64      `json:"next_fence"`
	ArtifactSHA256    string     `json:"artifact_sha256"`
	InventorySHA256   string     `json:"inventory_sha256"`
	ObservationSHA256 string     `json:"observation_sha256"`
	ObservedAt        *time.Time `json:"observed_at"`
	UnresolvedAttempt string     `json:"unresolved_attempt"`
}

type maintenanceReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func rollbackMaintenance(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func readTarget(ctx context.Context, reader maintenanceReader, owner, target string, lock bool) (Target, error) {
	query := `SELECT owner_id,target_id,profile_sha256,stopped,epoch,next_fence,COALESCE(artifact_sha256,''),COALESCE(inventory_sha256,''),COALESCE(observation_sha256,''),observed_at,COALESCE(unresolved_attempt,'') FROM maintenance_targets WHERE owner_id=$1 AND target_id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var result Target
	err := reader.QueryRow(ctx, query, owner, target).Scan(&result.OwnerID, &result.TargetID, &result.ProfileSHA256, &result.Stopped, &result.Epoch, &result.NextFence, &result.ArtifactSHA256, &result.InventorySHA256, &result.ObservationSHA256, &result.ObservedAt, &result.UnresolvedAttempt)
	return result, err
}

func readProfile(ctx context.Context, reader maintenanceReader, digest string) (Profile, error) {
	var raw []byte
	if !validDigest(digest) || reader.QueryRow(ctx, `SELECT raw FROM maintenance_profiles WHERE profile_sha256=$1`, digest).Scan(&raw) != nil || Digest(raw) != digest {
		return Profile{}, ErrState
	}
	return ParseProfile(raw)
}

func maintenanceEvent(ctx context.Context, tx pgx.Tx, target Target, kind, reference, digest string) error {
	_, err := tx.Exec(ctx, `INSERT INTO maintenance_events(owner_id,target_id,kind,reference,epoch,evidence_sha256) VALUES($1,$2,$3,$4,$5,NULLIF($6,''))`, target.OwnerID, target.TargetID, kind, reference, target.Epoch, digest)
	return err
}

// InstallProfile is a private operator operation. Installation never grants
// author access or starts a target; changed configuration invalidates observation
// and increments its stop epoch. Replayed identical installation is a no-op.
func (store Store) InstallProfile(ctx context.Context, raw []byte, previous string) (string, error) {
	profile, err := ParseProfile(raw)
	if err != nil || store.Pool == nil {
		return "", ErrState
	}
	digest := Digest(raw)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollbackMaintenance(tx)
	target, targetErr := readTarget(ctx, tx, profile.OwnerID, profile.TargetID, true)
	if targetErr == nil && target.ProfileSHA256 == digest {
		return digest, tx.Commit(ctx)
	}
	if targetErr != nil && !errors.Is(targetErr, pgx.ErrNoRows) {
		return "", ErrState
	}
	if targetErr == nil {
		old, err := readProfile(ctx, tx, target.ProfileSHA256)
		if err != nil || previous != target.ProfileSHA256 || profile.Revision <= old.Revision || target.UnresolvedAttempt != "" || (target.ArtifactSHA256 != "" && (profile.AnchorArtifactSHA256 != target.ArtifactSHA256 || profile.AnchorInventorySHA256 != target.InventorySHA256)) {
			return "", ErrState
		}
	} else if previous != "" {
		return "", ErrState
	}
	if _, err = tx.Exec(ctx, `INSERT INTO maintenance_profiles(profile_sha256,owner_id,target_id,raw) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, digest, profile.OwnerID, profile.TargetID, raw); err != nil {
		return "", err
	}
	if errors.Is(targetErr, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO maintenance_targets(owner_id,target_id,profile_sha256) VALUES($1,$2,$3)`, profile.OwnerID, profile.TargetID, digest)
	} else {
		_, err = tx.Exec(ctx, `UPDATE maintenance_targets SET profile_sha256=$3,stopped=true,epoch=epoch+1,observation_sha256=NULL,observed_at=NULL WHERE owner_id=$1 AND target_id=$2`, profile.OwnerID, profile.TargetID, digest)
	}
	if err != nil {
		return "", err
	}
	target, err = readTarget(ctx, tx, profile.OwnerID, profile.TargetID, false)
	if err != nil {
		return "", err
	}
	if err = maintenanceEvent(ctx, tx, target, "profile", digest, digest); err != nil {
		return "", err
	}
	return digest, tx.Commit(ctx)
}

func (store Store) Target(ctx context.Context, owner, target string) (Target, error) {
	if store.Pool == nil {
		return Target{}, ErrState
	}
	return readTarget(ctx, store.Pool, owner, target, false)
}

type stageSources struct {
	Before []SourceFile `json:"before"`
	After  []SourceFile `json:"after"`
}

// Stage compares submitted inventories against the current fixed profile. An
// independent build must attest their actual artifact binding. A staged
// artifact still needs actual node identity/compatibility, independent review and
// separate owner permission before any effect. A stopped target may be staged.
func (store Store) Stage(ctx context.Context, owner, targetID, profileDigest, base, candidate string, before, after []SourceFile) (string, error) {
	if store.Pool == nil || !validDigest(base) || !validDigest(candidate) || base == candidate {
		return "", ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil || target.ProfileSHA256 != profileDigest {
		return "", ErrState
	}
	profile, err := readProfile(ctx, tx, profileDigest)
	if err != nil {
		return "", err
	}
	diff, err := CompareInventory(before, after, profile.Boundaries)
	if err != nil {
		return "", err
	}
	expectedBase, expectedInventory := target.ArtifactSHA256, target.InventorySHA256
	if expectedBase == "" {
		expectedBase, expectedInventory = profile.AnchorArtifactSHA256, profile.AnchorInventorySHA256
	}
	if base != expectedBase || diff.BeforeSHA256 != expectedInventory {
		return "", ErrState
	}
	diffRaw, err := json.Marshal(diff)
	if err != nil {
		return "", err
	}
	binding := Binding{ProfileSHA256: profileDigest, BaseArtifactSHA256: base, CandidateSHA256: candidate, BeforeInventorySHA256: diff.BeforeSHA256, AfterInventorySHA256: diff.AfterSHA256, DiffSHA256: Digest(append([]byte("butler.maintenance.diff.v1\n"), diffRaw...)), Class: diff.Class, StateSchemaSHA256: profile.StateSchemaSHA256, ProtocolSHA256: profile.ProtocolSHA256, RuntimeSHA256: profile.RuntimeSHA256}
	bindingRaw, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	sources := stageSources{Before: append([]SourceFile(nil), before...), After: append([]SourceFile(nil), after...)}
	sort.Slice(sources.Before, func(i, j int) bool { return sources.Before[i].Path < sources.Before[j].Path })
	sort.Slice(sources.After, func(i, j int) bool { return sources.After[i].Path < sources.After[j].Path })
	sourceRaw, err := json.Marshal(sources)
	if err != nil || len(sourceRaw) > 32<<20 {
		return "", ErrState
	}
	stageID := "mstage_" + Digest(bindingRaw)
	insert, err := tx.Exec(ctx, `INSERT INTO maintenance_stages(stage_id,owner_id,target_id,profile_sha256,binding,source_inventory) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, stageID, owner, targetID, profileDigest, bindingRaw, sourceRaw)
	if err != nil {
		return "", err
	}
	if insert.RowsAffected() == 0 {
		var storedBinding, storedSources []byte
		if err = tx.QueryRow(ctx, `SELECT binding,source_inventory FROM maintenance_stages WHERE stage_id=$1`, stageID).Scan(&storedBinding, &storedSources); err != nil || !bytes.Equal(storedBinding, bindingRaw) || !bytes.Equal(storedSources, sourceRaw) {
			return "", ErrState
		}
	} else if err = maintenanceEvent(ctx, tx, target, "stage", stageID, Digest(bindingRaw)); err != nil {
		return "", err
	}
	return stageID, tx.Commit(ctx)
}

type ReviewReceipt struct {
	StageID                  string    `json:"stage_id"`
	ReviewSHA256             string    `json:"review_sha256"`
	ProfileSHA256            string    `json:"profile_sha256"`
	CurrentProfile           bool      `json:"current_profile"`
	EvidenceQualifiedAtWrite bool      `json:"evidence_qualified_at_write"`
	ObservedAt               time.Time `json:"observed_at"`
}

// SaveReview retains authentic negative/unknown/expired evidence as well as
// passing observations, including historical profiles. Qualification is checked
// again at an actual deployment claim; this receipt is not an executable grant.
func (store Store) SaveReview(ctx context.Context, owner, targetID, stageID string, raw, signature []byte) (ReviewReceipt, error) {
	if store.Pool == nil {
		return ReviewReceipt{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return ReviewReceipt{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return ReviewReceipt{}, ErrState
	}
	var profileDigest string
	var bindingRaw []byte
	if err = tx.QueryRow(ctx, `SELECT profile_sha256,binding FROM maintenance_stages WHERE stage_id=$1 AND owner_id=$2 AND target_id=$3`, stageID, owner, targetID).Scan(&profileDigest, &bindingRaw); err != nil || stageID != "mstage_"+Digest(bindingRaw) {
		return ReviewReceipt{}, ErrState
	}
	var binding Binding
	if json.Unmarshal(bindingRaw, &binding) != nil || !binding.valid() || binding.ProfileSHA256 != profileDigest {
		return ReviewReceipt{}, ErrState
	}
	profile, err := readProfile(ctx, tx, profileDigest)
	if err != nil {
		return ReviewReceipt{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return ReviewReceipt{}, err
	}
	policy := profile.ReviewPolicy()
	if _, err = InspectReview(raw, signature, binding, policy, now); err != nil {
		return ReviewReceipt{}, err
	}
	digest := Digest(raw)
	insert, err := tx.Exec(ctx, `INSERT INTO maintenance_reviews(review_sha256,stage_id,raw,signature) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, digest, stageID, raw, signature)
	if err != nil {
		return ReviewReceipt{}, err
	}
	if insert.RowsAffected() == 0 {
		var storedRaw, storedSignature []byte
		var storedStage string
		if err = tx.QueryRow(ctx, `SELECT stage_id,raw,signature FROM maintenance_reviews WHERE review_sha256=$1`, digest).Scan(&storedStage, &storedRaw, &storedSignature); err != nil || storedStage != stageID || !bytes.Equal(storedRaw, raw) || !bytes.Equal(storedSignature, signature) {
			return ReviewReceipt{}, ErrState
		}
	} else if err = maintenanceEvent(ctx, tx, target, "review", digest, digest); err != nil {
		return ReviewReceipt{}, err
	}
	_, qualificationErr := VerifyReview(raw, signature, binding, policy, now)
	receipt := ReviewReceipt{StageID: stageID, ReviewSHA256: digest, ProfileSHA256: profileDigest, CurrentProfile: target.ProfileSHA256 == profileDigest, EvidenceQualifiedAtWrite: qualificationErr == nil, ObservedAt: now}
	return receipt, tx.Commit(ctx)
}

// Stop is independent of Core and invalidates future node dispatch through the
// durable epoch. An already dispatched physical effect stays completed/unknown;
// this method does not invent a filesystem, container or external-effect undo.
func (store Store) Stop(ctx context.Context, owner, targetID string) (Target, error) {
	if store.Pool == nil {
		return Target{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return Target{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return Target{}, ErrState
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET stopped=true,epoch=epoch+1,observation_sha256=NULL,observed_at=NULL WHERE owner_id=$1 AND target_id=$2`, owner, targetID); err != nil {
		return Target{}, err
	}
	target.Epoch++
	target.Stopped = true
	target.ObservationSHA256, target.ObservedAt = "", nil
	if err = maintenanceEvent(ctx, tx, target, "stop", fmt.Sprintf("stop:%d", target.Epoch), ""); err != nil {
		return Target{}, err
	}
	return target, tx.Commit(ctx)
}

// Resume requires a separately authenticated owner operation and a fresh actual
// node observation. It cannot clear an unresolved effect or grant deployment.
func (store Store) Resume(ctx context.Context, owner, targetID, profileDigest string, expectedEpoch int64) (Target, error) {
	if store.Pool == nil {
		return Target{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return Target{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil || target.ProfileSHA256 != profileDigest || target.Epoch != expectedEpoch || !target.Stopped || target.UnresolvedAttempt != "" || target.ArtifactSHA256 == "" || target.InventorySHA256 == "" || target.ObservationSHA256 == "" || target.ObservedAt == nil {
		return Target{}, ErrState
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Target{}, err
	}
	if target.ObservedAt.After(now) || now.Sub(*target.ObservedAt) > 30*time.Second {
		return Target{}, ErrState
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET stopped=false,epoch=epoch+1,observation_sha256=NULL,observed_at=NULL WHERE owner_id=$1 AND target_id=$2`, owner, targetID); err != nil {
		return Target{}, err
	}
	target.Epoch++
	target.Stopped = false
	target.ObservationSHA256, target.ObservedAt = "", nil
	if err = maintenanceEvent(ctx, tx, target, "resume", fmt.Sprintf("resume:%d", target.Epoch), ""); err != nil {
		return Target{}, err
	}
	return target, tx.Commit(ctx)
}

type ObservationReceipt struct {
	ObservationSHA256 string `json:"observation_sha256"`
	Applied           bool   `json:"applied"`
	KnownArtifact     bool   `json:"known_artifact"`
}

// RecordObservation is called only by the private controller's node client,
// with its original nonce/epoch, after a signed fixed-target inspection. Old or
// superseded observations remain immutable history and never refresh readiness.
func (store Store) RecordObservation(ctx context.Context, owner, targetID, profileDigest, nonce string, epoch int64, raw, signature []byte) (ObservationReceipt, error) {
	if store.Pool == nil {
		return ObservationReceipt{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return ObservationReceipt{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return ObservationReceipt{}, ErrState
	}
	profile, err := readProfile(ctx, tx, profileDigest)
	if err != nil || profile.OwnerID != owner || profile.TargetID != targetID {
		return ObservationReceipt{}, ErrState
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return ObservationReceipt{}, err
	}
	observation, err := InspectNodeObservation(raw, signature, profile, profileDigest, nonce, epoch, now)
	if err != nil {
		return ObservationReceipt{}, err
	}
	digest := Digest(raw)
	insert, err := tx.Exec(ctx, `INSERT INTO maintenance_observations(observation_sha256,owner_id,target_id,profile_sha256,nonce,raw,signature,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, digest, owner, targetID, profileDigest, nonce, raw, signature, observation.ObservedAt)
	if err != nil {
		return ObservationReceipt{}, err
	}
	if insert.RowsAffected() == 0 {
		var storedRaw, storedSignature []byte
		if err = tx.QueryRow(ctx, `SELECT raw,signature FROM maintenance_observations WHERE owner_id=$1 AND target_id=$2 AND nonce=$3`, owner, targetID, nonce).Scan(&storedRaw, &storedSignature); err != nil || !bytes.Equal(storedRaw, raw) || !bytes.Equal(storedSignature, signature) {
			return ObservationReceipt{}, ErrState
		}
		return ObservationReceipt{ObservationSHA256: digest}, tx.Commit(ctx)
	}
	current := target.ProfileSHA256 == profileDigest && target.Epoch == epoch && now.Sub(observation.ObservedAt) <= 30*time.Second && (target.ObservedAt == nil || observation.ObservedAt.After(*target.ObservedAt))
	knownRoot := ""
	if observation.State != "missing" && observation.RuntimeSHA256 == profile.RuntimeSHA256 {
		if observation.ArtifactSHA256 == target.ArtifactSHA256 && target.InventorySHA256 != "" {
			knownRoot = target.InventorySHA256
		} else if observation.ArtifactSHA256 == profile.AnchorArtifactSHA256 {
			knownRoot = profile.AnchorInventorySHA256
		}
	}
	// A different artifact is retained as an unqualified physical observation.
	// Mapping a new candidate requires the independently admitted actual-effect
	// path, which is not replaced by a node's image-name assertion.
	if current {
		artifact := observation.ArtifactSHA256
		if knownRoot == "" {
			artifact = ""
		}
		if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET artifact_sha256=NULLIF($3,''),inventory_sha256=NULLIF($4,''),observation_sha256=$5,observed_at=$6 WHERE owner_id=$1 AND target_id=$2`, owner, targetID, artifact, knownRoot, digest, observation.ObservedAt); err != nil {
			return ObservationReceipt{}, err
		}
	}
	if err = maintenanceEvent(ctx, tx, target, "observation", digest, digest); err != nil {
		return ObservationReceipt{}, err
	}
	return ObservationReceipt{ObservationSHA256: digest, Applied: current, KnownArtifact: current && knownRoot != ""}, tx.Commit(ctx)
}
