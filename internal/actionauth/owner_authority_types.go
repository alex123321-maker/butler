package actionauth

import "time"

// Assignment is service-owned authority, never a model tool argument. Its
// purpose comes from a confirmed owner goal and an explicitly selected mandate.
type Assignment struct {
	TaskID           string `json:"task_id"`
	OwnerID          string `json:"owner_id"`
	GoalID           string `json:"goal_id"`
	GoalRevision     int64  `json:"goal_revision"`
	MandateID        string `json:"mandate_id"`
	MandateRevision  int64  `json:"mandate_revision"`
	Purpose          string `json:"purpose"`
	InitiativeReason string `json:"initiative_reason"`
	ObservationRef   string `json:"observation_ref"`
}

type MandateBudget struct {
	ModelTurns         int64 `json:"model_turns"`
	Tokens             int64 `json:"tokens"`
	CostMicros         int64 `json:"cost_micros"`
	ExternalCalls      int64 `json:"external_calls"`
	Children           int64 `json:"children"`
	ActiveMillis       int64 `json:"active_millis"`
	EgressBytes        int64 `json:"egress_bytes"`
	ProviderInputBytes int64 `json:"provider_input_bytes"`
}

type ActiveHours struct {
	Timezone    string `json:"timezone"`
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
	Weekdays    []int  `json:"weekdays"`
}

type ResourceScope struct {
	Origin     string `json:"origin"`
	PathPrefix string `json:"path_prefix"`
}

// ExtensionPermission approves attachment only of an exact reviewed manifest
// and capability subset. It never approves arbitrary code or runtime effects.
type ExtensionPermission struct {
	Source       string   `json:"source"`
	ExtensionID  string   `json:"extension_id"`
	ManifestHash string   `json:"manifest_hash"`
	Tools        []string `json:"tools"`
}

// GoalPolicy is the typed durable owner projection shared with authorization.
// Assumptions remain proposed data and cannot supply this permission boundary.
type GoalPolicy struct {
	ID          string    `json:"goal_id"`
	OwnerID     string    `json:"owner_id"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	Revision    int64     `json:"revision"`
	Title       string    `json:"title"`
	Purpose     string    `json:"purpose"`
	Constraints []string  `json:"constraints"`
	ExpiresAt   time.Time `json:"expires_at"`
}
