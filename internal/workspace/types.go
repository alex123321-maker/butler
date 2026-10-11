// Package workspace owns bounded isolated code files. It grants no execution
// authority; the kernel must supply a same-transaction ownership/resource guard.
package workspace

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrDenied         = errors.New("workspace access denied")
	ErrConflict       = errors.New("workspace file version conflict")
	ErrDrift          = errors.New("workspace physical state differs from committed metadata")
	ErrOutcomeUnknown = errors.New("workspace persistence outcome unknown; owner reconciliation required")
)

// Binding and LeaseProof are private kernel metadata, never model arguments.
// A workspace survives runs but remains pinned to a particular task revision.
type Binding struct {
	OwnerID             string `json:"-"`
	SessionKey          string `json:"-"`
	TaskID              string `json:"-"`
	TaskRevision        int64  `json:"-"`
	ConstraintsRevision int64  `json:"-"`
}
type LeaseProof struct {
	LeaseID                 string `json:"-"`
	ResourceID              string `json:"-"`
	FenceEpoch              int64  `json:"-"`
	ExpectedResourceVersion int64  `json:"-"`
	RunID                   string `json:"-"`
	SessionKey              string `json:"-"`
	TaskID                  string `json:"-"`
}
type Ref struct {
	ID      string  `json:"workspace_id"`
	Binding Binding `json:"-"`
}
type Access struct {
	Ref       Ref
	Proof     LeaseProof
	Operation string // create, read, write; never inferred from a tool name
	Path      string
	Bytes     int64
}

// CheckTx must lock/check live ownership -> session/task/goal -> sorted resource
// authority before workspace rows. It must recheck current DB time when invoked
// again while waiting on workspace locks. Create/write must lock resource rows
// for update on the first call, rather than upgrading shared locks after a wait.
// No second pooled connection is needed.
type Guard interface {
	CheckTx(context.Context, pgx.Tx, Access) error
	// ConfirmWriteTx rechecks current authority and records resource observation
	// and lease consumption on this transaction, after the filesystem rename and
	// metadata CAS. Failure is an uncertain physical effect, never a rolled-back
	// file. An adapter must not confirm a model-supplied snapshot or defer this
	// write to another transaction.
	ConfirmWriteTx(context.Context, pgx.Tx, Access, Snapshot) error
}

type Snapshot struct {
	Path    string `json:"path"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
}
type File struct {
	Snapshot
	Content []byte `json:"-"`
}
type Limits struct {
	MaxFileBytes      int64
	MaxWorkspaceBytes int64
	MaxFiles          int
}

func DefaultLimits() Limits { return Limits{1 << 20, 16 << 20, 256} }

func ResourceID(id string) string { return "workspace:" + id }
