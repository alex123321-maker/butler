package readcontrol

// TemporalEvidence is an optional accepted-claim observation, populated only by
// the owning #88 catalog service. It is separate from Item.Freshness (current
// source revision) and from source EventTime. Catalog authority/pins never come
// from model input. This DTO intentionally has no dependency on catalog.
type TemporalEvidence struct {
	ClaimID           string            `json:"claim_id"`
	Validity          string            `json:"validity"`
	ValidFrom         *string           `json:"valid_from"`
	ValidTo           *string           `json:"valid_to"`
	RecordedAt        *string           `json:"recorded_at"`
	SourceRecordedAt  string            `json:"source_recorded_at"`
	AcceptedAt        string            `json:"accepted_at"`
	FreshnessClass    string            `json:"freshness_class"`
	VerifiedAt        *string           `json:"verified_at"`
	VerificationBasis string            `json:"verification_basis"`
	Freshness         TemporalFreshness `json:"freshness"`
	Historical        bool              `json:"historical"`
	TemporalRevision  int64             `json:"temporal_revision"`
}

type TemporalFreshness struct {
	Status               string  `json:"status"`
	RevalidateAfter      *string `json:"revalidate_after"`
	RequiresRevalidation bool    `json:"requires_revalidation"`
	CurrentUsable        bool    `json:"current_usable"`
}
