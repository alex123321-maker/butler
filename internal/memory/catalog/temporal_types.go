package catalog

import "time"

// TemporalQuery keeps claimed validity independent of when Butler knew a claim.
// Nil/nil selects the current owner head. Neither date grants source access.
type TemporalQuery struct {
	ValidAt *time.Time `json:"valid_at,omitempty"`
	KnownAt *time.Time `json:"known_at,omitempty"`
}

type Validity string

const (
	ValidityUnknown  Validity = "unknown"
	ValidityInterval Validity = "interval"
)

type FreshnessClass string

const (
	FreshnessUnknown          FreshnessClass = "unknown"
	FreshnessStablePreference FreshnessClass = "stable_preference"
	FreshnessWorkStatus       FreshnessClass = "work_status"
	FreshnessTechnicalFact    FreshnessClass = "technical_fact"
	FreshnessCompletedEpisode FreshnessClass = "completed_episode"
)

// TemporalMetadata versions owner-supplied interpretation of the original claim.
// Unknown validity requires null bounds. With explicit interval validity a null
// bound is open. Raw relative wording is retained in the claim, never parsed here.
// SourceRecordedAt reuses the source revision's extracted_at ingestion timestamp;
// RecordedAt is this metadata revision's core-assigned recording time.
type TemporalMetadata struct {
	ClaimID                  string         `json:"claim_id"`
	Revision                 int64          `json:"revision"`
	Validity                 Validity       `json:"validity"`
	ValidFrom                *time.Time     `json:"valid_from"`
	ValidTo                  *time.Time     `json:"valid_to"`
	RecordedAt               *time.Time     `json:"recorded_at"`
	SourceRecordedAt         time.Time      `json:"source_recorded_at"`
	AcceptedAt               *time.Time     `json:"accepted_at"`
	Class                    FreshnessClass `json:"freshness_class"`
	VerifiedAt               *time.Time     `json:"verified_at"`
	VerificationBasis        string         `json:"verification_basis"`
	VerificationActionID     string         `json:"verification_action_id,omitempty"`
	VerificationEvidenceID   string         `json:"verification_evidence_id,omitempty"`
	VerificationResourceHash string         `json:"verification_resource_hash,omitempty"`
}

// Freshness describes retrieval suitability; it never changes an owner decision
// or removes historical evidence. A current-source pin alone is not freshness.
type Freshness struct {
	Status               string     `json:"status"`
	RevalidateAfter      *time.Time `json:"revalidate_after"`
	RequiresRevalidation bool       `json:"requires_revalidation"`
	CurrentUsable        bool       `json:"current_usable"`
}

// AcceptedFactPin is content-free. All fields are rechecked against the current
// claim/head, exact source proof, temporal revision and time-sensitive freshness.
// Historical reads still pin the current head to fence subsequent owner changes.
// TemporalRevision pins the current metadata head; TemporalMetadata.Revision can
// be older when a KnownAt query selects an earlier recorded interpretation.
type AcceptedFactPin struct {
	ClaimID          string    `json:"claim_id"`
	HeadRevision     int64     `json:"head_revision"`
	ActiveID         string    `json:"active_id"`
	TemporalRevision int64     `json:"temporal_revision"`
	Source           SourceRef `json:"source"`
	MetadataHash     string    `json:"metadata_hash"`
	FreshnessStatus  string    `json:"freshness_status"`
	CurrentUsable    bool      `json:"current_usable"`
}

type AcceptedFact struct {
	Claim      Claim            `json:"claim"`
	Temporal   TemporalMetadata `json:"temporal"`
	Freshness  Freshness        `json:"freshness"`
	Pin        AcceptedFactPin  `json:"pin"`
	Historical bool             `json:"historical"`
	Conflict   bool             `json:"conflict"`
}

// AcceptedFactPage reports unknown exclusions explicitly. A dated query does not
// silently interpret an absent validity interval as matching that date.
type AcceptedFactPage struct {
	Items                 []AcceptedFact `json:"items"`
	UnknownValidity       int            `json:"unknown_validity"`
	NotKnownAtObservation int            `json:"not_known_at_observation"`
	Truncated             bool           `json:"truncated"`
}

// TemporalObservation exposes only metadata explicitly bound to a derivative,
// never additional parent values/texts. OnSourceID is the selected derivative;
// Source remains the exact original owner-accepted claim provenance.
type TemporalObservation struct {
	OnSourceID string           `json:"on_source_id"`
	ClaimID    string           `json:"claim_id"`
	Source     SourceRef        `json:"source"`
	Temporal   TemporalMetadata `json:"temporal"`
	Freshness  Freshness        `json:"freshness"`
	Historical bool             `json:"historical"`
}
type TemporalObservationPage struct {
	Items     []TemporalObservation `json:"items"`
	Truncated bool                  `json:"truncated"`
}

// TemporalUpdate is an authenticated owner control, not model extraction output.
// Verification timestamps from Broker reads use a separate trusted receipt path.
type TemporalUpdate struct {
	ClaimID              string         `json:"claim_id"`
	ExpectedRevision     int64          `json:"expected_revision"`
	ExpectedHeadRevision int64          `json:"expected_head_revision"`
	Validity             Validity       `json:"validity"`
	ValidFrom            *time.Time     `json:"valid_from"`
	ValidTo              *time.Time     `json:"valid_to"`
	Class                FreshnessClass `json:"freshness_class"`
	OwnerVerifiedNow     bool           `json:"owner_verified_now"`
}

// BrokerTemporalUpdate records an explicit owner's same-fact review of an
// already completed, budgeted Broker read. It cannot initiate network traffic.
// A read receipt proves observation, not semantic truth without that review.
type BrokerTemporalUpdate struct {
	ClaimID              string `json:"claim_id"`
	ExpectedRevision     int64  `json:"expected_revision"`
	ExpectedHeadRevision int64  `json:"expected_head_revision"`
	EvidenceID           string `json:"evidence_id"`
	ExpectedResourceHash string `json:"expected_resource_hash"`
	ConfirmedSameFact    bool   `json:"confirmed_same_fact"`
}
