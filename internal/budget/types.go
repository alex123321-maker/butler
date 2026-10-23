// Package budget owns durable resource limits and conservative reservations.
// Callers supply trusted scope/operation identities, never model arguments.
package budget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid execution budget request")
	ErrConflict    = errors.New("execution budget identity or policy conflict")
	ErrUnknownCost = errors.New("execution budget requires a trusted cost quote")
	ErrNotFound    = errors.New("execution budget scope not found")
)

type Units struct {
	ModelTurns         int64 `json:"model_turns"`
	Tokens             int64 `json:"tokens"`
	CostMicros         int64 `json:"cost_micros"`
	ExternalCalls      int64 `json:"external_calls"`
	Children           int64 `json:"children"`
	ActiveMillis       int64 `json:"active_millis"`
	EgressBytes        int64 `json:"egress_bytes"`
	ProviderInputBytes int64 `json:"provider_input_bytes"`
}

type ScopeSpec struct {
	Kind      string    `json:"kind"`
	OwnerID   string    `json:"owner_id"`
	SubjectID string    `json:"subject_id"`
	Limits    Units     `json:"limits"`
	Deadline  time.Time `json:"deadline"`
}

type Scope struct {
	ID string `json:"scope_id"`
	ScopeSpec
	Used Units `json:"used"`
}

type Request struct {
	OwnerID     string   `json:"owner_id"`
	OperationID string   `json:"operation_id"`
	ScopeIDs    []string `json:"scope_ids"`
	Charge      Units    `json:"charge"`
	CostKnown   bool     `json:"cost_known"`
}

type Reservation struct {
	ID string `json:"reservation_id"`
	Request
	CreatedAt time.Time `json:"created_at"`
}

type ExhaustedError struct{ Kind, Dimension string }

func (e *ExhaustedError) Error() string {
	return "execution budget exhausted: " + e.Dimension + " (" + e.Kind + ")"
}

func ScopeID(kind, owner, subject string) string {
	return digest([]string{kind, owner, subject})
}
func digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (u Units) validate() error {
	for _, amount := range u.values() {
		if amount < 0 {
			return ErrInvalid
		}
	}
	return nil
}
func (u Units) values() [8]int64 {
	return [8]int64{u.ModelTurns, u.Tokens, u.CostMicros, u.ExternalCalls, u.Children, u.ActiveMillis, u.EgressBytes, u.ProviderInputBytes}
}
func fromValues(v [8]int64) Units { return Units{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]} }
func addWithin(scope Scope, charge Units, now time.Time) (Units, error) {
	return addWithinPolicy(scope, charge, now, false)
}

// The operator may disable only the conversation model-turn comparison.
// Original limits, every charge and monotonic spending remain durable. Other
// dimensions/scopes, deadlines and integer overflow still fail closed.
func addWithinPolicy(scope Scope, charge Units, now time.Time, disableConversationModelTurns bool) (Units, error) {
	if scope.Used.validate() != nil || scope.Limits.validate() != nil || charge.validate() != nil {
		return Units{}, ErrInvalid
	}
	if !scope.Deadline.After(now) {
		return Units{}, &ExhaustedError{scope.Kind, "deadline"}
	}
	used, limits, amounts := scope.Used.values(), scope.Limits.values(), charge.values()
	names := [8]string{"model_turns", "tokens", "cost_micros", "external_calls", "children", "active_millis", "egress_bytes", "provider_input_bytes"}
	for i, amount := range amounts {
		if i == 0 && scope.Kind == "conversation" && disableConversationModelTurns {
			if amount > math.MaxInt64-used[i] {
				return Units{}, ErrInvalid
			}
			used[i] += amount
			continue
		}
		// Subtraction avoids integer overflow even for an untrusted large quote.
		if used[i] > limits[i] || amount > limits[i]-used[i] {
			return Units{}, &ExhaustedError{scope.Kind, names[i]}
		}
		used[i] += amount
	}
	return fromValues(used), nil
}
func validateScope(spec ScopeSpec) error {
	if spec.OwnerID == "" || spec.SubjectID == "" || len(spec.OwnerID) > 512 || len(spec.SubjectID) > 512 || spec.Deadline.IsZero() {
		return ErrInvalid
	}
	switch spec.Kind {
	case "task", "mandate", "owner_day", "conversation", "auxiliary", "delegation":
	default:
		return ErrInvalid
	}
	return spec.Limits.validate()
}
func canonicalRequest(req Request) (Request, error) {
	if strings.TrimSpace(req.OwnerID) == "" || len(req.OwnerID) > 512 || strings.TrimSpace(req.OperationID) == "" || len(req.OperationID) > 512 || len(req.ScopeIDs) == 0 || len(req.ScopeIDs) > 16 {
		return Request{}, ErrInvalid
	}
	if err := req.Charge.validate(); err != nil {
		return Request{}, err
	}
	if !req.CostKnown {
		return Request{}, ErrUnknownCost
	}
	req.ScopeIDs = append([]string(nil), req.ScopeIDs...)
	sort.Strings(req.ScopeIDs)
	for i, id := range req.ScopeIDs {
		if len(id) != 64 {
			return Request{}, ErrInvalid
		}
		if _, err := hex.DecodeString(id); err != nil {
			return Request{}, fmt.Errorf("%w: invalid scope identity", ErrInvalid)
		}
		if i > 0 && id == req.ScopeIDs[i-1] {
			return Request{}, ErrInvalid
		}
	}
	return req, nil
}
