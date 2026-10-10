// Package goal owns confirmed owner intent and durable broad autonomy decisions.
// Proposed assumptions are data. Only authenticated owner decisions can create
// authority, and task assignments pin the exact reviewed revisions.
package goal

import (
	"errors"
	"github.com/butler/butler/internal/actionauth"
	"time"
)

var (
	ErrInvalid      = errors.New("invalid owner authority request")
	ErrConflict     = errors.New("owner authority revision or delivery conflict")
	ErrUnauthorized = errors.New("authenticated owner authority required")
)

type Source struct {
	Author     string `json:"author"`
	Channel    string `json:"channel"`
	DeliveryID string `json:"delivery_id"`
	ParentRef  string `json:"parent_ref,omitempty"`
}

type Record struct {
	actionauth.GoalPolicy
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GoalRequest struct {
	Goal             actionauth.GoalPolicy `json:"goal"`
	ExpectedRevision int64                 `json:"expected_revision"`
	ParentRef        string                `json:"parent_ref,omitempty"`
}

type MandateRequest struct {
	Mandate          actionauth.Mandate `json:"mandate"`
	ExpectedRevision int64              `json:"expected_revision"`
}

type Receipt struct {
	EventID     string `json:"event_id"`
	SubjectID   string `json:"subject_id"`
	Revision    int64  `json:"revision"`
	Propagation string `json:"propagation"`
}

type AdmissionRequest struct {
	OwnerID          string
	GoalID           string
	MandateID        string
	Purpose          string
	ObservationRef   string
	InitiativeReason string
}

type Obligation struct {
	actionauth.Assignment
	TaskStatus       string `json:"task_status"`
	TaskRevision     int64  `json:"task_revision"`
	AuthorityCurrent bool   `json:"authority_current"`
}

type View struct {
	Goals              []Record             `json:"goals"`
	Mandates           []actionauth.Mandate `json:"mandates"`
	Obligations        []Obligation         `json:"obligations"`
	PendingPropagation int                  `json:"pending_propagation"`
}
