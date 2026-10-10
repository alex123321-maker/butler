// Package initiative owns durable observations, schedules and proposed useful work.
// An observation is data; only current owner authority can admit its execution.
package initiative

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Alpine deployments must retain IANA rules without host files.
)

var (
	ErrInvalid  = errors.New("invalid initiative input")
	ErrConflict = errors.New("initiative identity conflict")
	ErrNotFound = errors.New("initiative not found")
)

var identifier = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_.:/-]{0,199}$")
var digest = regexp.MustCompile("^[a-f0-9]{64}$")

type Source struct {
	ID                  string   `json:"source_id"`
	OwnerID             string   `json:"owner_id"`
	GoalID              string   `json:"goal_id"`
	MandateID           string   `json:"mandate_id"`
	SessionKey          string   `json:"session_key"`
	ProjectRef          string   `json:"project_ref"`
	Purpose             string   `json:"purpose"`
	TimeZone            string   `json:"time_zone"`
	TaskGoal            string   `json:"task_goal"`
	ExecutionPolicy     string   `json:"execution_policy"`
	CompletionCriteria  []string `json:"completion_criteria"`
	ActiveStart         int      `json:"active_start_hour"`
	ActiveEnd           int      `json:"active_end_hour"`
	FailureThreshold    int      `json:"failure_threshold"`
	ConfidenceThreshold int      `json:"confidence_threshold"`
	MinimumImportance   int      `json:"minimum_importance"`
	CooldownSeconds     int      `json:"cooldown_seconds"`
	Enabled             bool     `json:"enabled"`
	Revision            int64    `json:"revision"`
}

func (s Source) Validate() error {
	for _, v := range []string{s.ID, s.OwnerID, s.GoalID, s.MandateID, s.SessionKey, s.ProjectRef, s.Purpose} {
		if !identifier.MatchString(v) {
			return ErrInvalid
		}
	}
	if _, err := time.LoadLocation(s.TimeZone); err != nil || s.TimeZone == "" || s.TimeZone == "Local" {
		return ErrInvalid
	}
	if strings.TrimSpace(s.TaskGoal) == "" || len(s.TaskGoal) > 8192 || len(s.CompletionCriteria) == 0 || len(s.CompletionCriteria) > 16 {
		return ErrInvalid
	}
	if s.ExecutionPolicy != "prepare_only" && s.ExecutionPolicy != "normal" {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, criterion := range s.CompletionCriteria {
		if strings.TrimSpace(criterion) == "" || len(criterion) > 1024 || seen[criterion] {
			return ErrInvalid
		}
		seen[criterion] = true
	}
	if s.ActiveStart < 0 || s.ActiveStart > 23 || s.ActiveEnd < 0 || s.ActiveEnd > 23 || s.ActiveStart == s.ActiveEnd ||
		s.FailureThreshold < 1 || s.FailureThreshold > 100 || s.ConfidenceThreshold < 1 || s.ConfidenceThreshold > 100 || s.MinimumImportance < 0 || s.MinimumImportance > 100 ||
		s.CooldownSeconds < 60 || s.CooldownSeconds > 7*24*3600 {
		return ErrInvalid
	}
	return nil
}

// Event contains a typed observation and hashes, never a raw error, URL or secret.
// The observer subsystem supplies ProducedByButler; it is not model metadata.
type Event struct {
	ID               string    `json:"event_id"`
	SourceID         string    `json:"source_id"`
	Kind             string    `json:"kind"`
	ConditionDigest  string    `json:"condition_digest"`
	ResourceRevision string    `json:"resource_revision"`
	EvidenceDigest   string    `json:"evidence_digest"`
	Confidence       int       `json:"confidence"`
	Importance       int       `json:"importance"`
	ProducedByButler bool      `json:"produced_by_butler"`
	ObservedAt       time.Time `json:"observed_at"`
}

func (e Event) Validate() error {
	if !identifier.MatchString(e.ID) || !identifier.MatchString(e.SourceID) ||
		!digest.MatchString(e.ConditionDigest) || !digest.MatchString(e.ResourceRevision) ||
		!digest.MatchString(e.EvidenceDigest) || e.ObservedAt.IsZero() ||
		e.Confidence < 0 || e.Confidence > 100 || e.Importance < 0 || e.Importance > 100 {
		return ErrInvalid
	}
	switch e.Kind {
	case "project_error", "project_healthy", "service_change", "deadline", "task_completed", "periodic":
		return nil
	default:
		return ErrInvalid
	}
}

type Candidate struct {
	ID                 string    `json:"initiative_id"`
	SourceID           string    `json:"source_id"`
	SourceRevision     int64     `json:"source_revision"`
	OwnerID            string    `json:"owner_id"`
	GoalID             string    `json:"goal_id"`
	MandateID          string    `json:"mandate_id"`
	Fingerprint        string    `json:"fingerprint"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	Occurrences        int       `json:"occurrences"`
	Confidence         int       `json:"confidence"`
	Importance         int       `json:"importance"`
	ExpectedModelCalls int       `json:"expected_model_calls"`
	AttentionCost      int       `json:"attention_cost"`
	NotBefore          time.Time `json:"not_before"`
	LastObservedAt     time.Time `json:"last_observed_at"`
	TaskID             string    `json:"task_id,omitempty"`
	InputID            string    `json:"input_id,omitempty"`
	ObservationRef     string    `json:"observation_ref"`
}

func hash(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func (e Event) fingerprint(s Source) string {
	return hash([]string{s.OwnerID, s.GoalID, s.MandateID, s.Purpose, s.ProjectRef, e.Kind, e.ConditionDigest, e.ResourceRevision})
}

func active(s Source, at time.Time) bool {
	loc, _ := time.LoadLocation(s.TimeZone)
	hour := at.In(loc).Hour()
	if s.ActiveStart < s.ActiveEnd {
		return hour >= s.ActiveStart && hour < s.ActiveEnd
	}
	return hour >= s.ActiveStart || hour < s.ActiveEnd
}

// Decide uses deterministic filters. A low confidence observation never admits
// a model or external action. Execution still requires separate mandate/budgets.
func Decide(s Source, e Event, occurrences int, at time.Time) (status, reason string, notBefore time.Time) {
	if s.Validate() != nil || e.Validate() != nil || occurrences < 0 || at.IsZero() {
		return "rejected", "invalid_observation", at
	}
	if !s.Enabled {
		return "rejected", "source_disabled", at
	}
	if e.ProducedByButler {
		return "suppressed", "own_change", at
	}
	if e.Kind == "project_healthy" {
		return "suppressed", "condition_cleared", at
	}
	if e.Confidence < s.ConfidenceThreshold {
		return "proposed", "confidence_insufficient", at
	}
	if e.Kind == "project_error" && occurrences < s.FailureThreshold {
		return "observing", "repeat_threshold_not_met", at
	}
	if e.Kind == "periodic" {
		return "observing", "empty_check", at
	}
	if e.Importance < s.MinimumImportance {
		return "proposed", "importance_insufficient", at
	}
	if !active(s, at) {
		return "deferred", "outside_active_hours", nextActive(s, at)
	}
	return "ready", "useful_observation", at
}

func nextActive(s Source, at time.Time) time.Time {
	loc, _ := time.LoadLocation(s.TimeZone)
	local := at.In(loc)
	for day := 0; day < 8; day++ {
		date := local.AddDate(0, 0, day)
		candidates := localInstants(date.Year(), date.Month(), date.Day(), s.ActiveStart, 0, loc)
		for _, candidate := range candidates {
			if candidate.After(at) {
				return candidate
			}
		}
	}
	return at.Add(24 * time.Hour)
}
