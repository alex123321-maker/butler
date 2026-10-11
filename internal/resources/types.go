// Package resources arbitrates finite leases for exact kernel resource IDs.
// It owns only its PostgreSQL rows; callers retain task, effect and budget policy.
package resources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrDenied   = errors.New("resource authority is unavailable or stale")
	ErrConflict = errors.New("resource is held, changed or requires reconciliation")
)

type Mode string

const (
	Read  Mode = "read"
	Write Mode = "write"
)

// Binding and Proof are private kernel inputs, never JSON/model tool arguments.
type Binding struct {
	OwnerID             string    `json:"-"`
	RunID               string    `json:"-"`
	SessionKey          string    `json:"-"`
	TaskID              string    `json:"-"`
	TaskRevision        int64     `json:"-"`
	ConstraintsRevision int64     `json:"-"`
	LeaseEpoch          int64     `json:"-"`
	Deadline            time.Time `json:"-"`
}

type Proof struct {
	LeaseID                 string `json:"-"`
	ResourceID              string `json:"-"`
	FenceEpoch              int64  `json:"-"`
	ExpectedResourceVersion int64  `json:"-"`
	RunID                   string `json:"-"`
	SessionKey              string `json:"-"`
	TaskID                  string `json:"-"`
}

type Request struct {
	ResourceID      string
	Mode            Mode
	ExpectedVersion int64
}

// Guard locks and checks live execution/task/owner policy in the caller's
// transaction before resource locks. Repeated calls must check the same rows
// without borrowing another connection or acquiring a different authority.
type Guard interface {
	CheckTx(context.Context, pgx.Tx, Binding) error
}

// OwnerGuard accepts only authenticated owner controls and independently
// obtained effect/artifact evidence for this exact lease, never a model verdict.
// It locks relevant authority/evidence rows before resource rows on this same
// transaction. Reconciliation is separate from execution authority.
type OwnerGuard interface {
	CheckReconciliationTx(context.Context, pgx.Tx, Decision) error
}

type Decision struct {
	OwnerID         string
	ResourceID      string
	LeaseID         string
	EvidenceID      string
	ExpectedVersion int64
}

func validBinding(b Binding) bool {
	return b.OwnerID != "" && b.RunID != "" && b.SessionKey != "" && b.TaskID != "" && b.TaskRevision > 0 && b.ConstraintsRevision > 0 && b.LeaseEpoch > 0 && !b.Deadline.IsZero()
}

func validID(id string) bool {
	parts := strings.Split(id, ":")
	if len(parts) != 2 || len(parts[1]) < 1 || len(parts[1]) > 128 {
		return false
	}
	switch parts[0] {
	case "workspace", "browser-tab", "memory":
	default:
		return false
	}
	for _, c := range parts[1] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return parts[1] != "." && parts[1] != ".."
}

func validProof(b Binding, p Proof) bool {
	return validBinding(b) && validID(p.ResourceID) && p.LeaseID != "" && p.FenceEpoch > 0 && p.ExpectedResourceVersion > 0 && p.RunID == b.RunID && p.SessionKey == b.SessionKey && p.TaskID == b.TaskID
}
