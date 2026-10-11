// Package singletabcontrol owns guarded owner selection, not tool execution.
// Its guard and source checks run in the same caller transaction before every
// candidate/approval/binding lock. Runtime observations grant no selection rights.
package singletabcontrol

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = errors.New("current guarded owner tab selection required")

const MaxTabs = 64
const BindTool = "single_tab.bind.guarded"

// Tab is private fixed host metadata. It contains no title, URL path/query,
// document, cookies or arbitrary extension payload. Only tokens and origin
// are released to the authenticated owner by Selection.
type Tab struct {
	BrowserInstance string `json:"browser_instance_id"`
	Ref             string `json:"tab_ref"`
	Origin          string `json:"origin"`
	Active          bool   `json:"active"`
}
type Metadata interface {
	List(context.Context) ([]Tab, error)
	Check(context.Context, Tab) error
}
type Candidate struct {
	Token   string `json:"candidate_token"`
	Origin  string `json:"origin"`
	Ordinal int    `json:"ordinal"`
	Active  bool   `json:"active"`
}
type Selection struct {
	ApprovalID string      `json:"approval_id"`
	Candidates []Candidate `json:"candidates"`
	ExpiresAt  time.Time   `json:"expires_at"`
}
type Controller struct {
	pool     *pgxpool.Pool
	metadata Metadata
	guard    resources.Guard
	sources  resourceexec.SourceGuard
	host     string
}

func New(pool *pgxpool.Pool, metadata Metadata, guard resources.Guard, sources resourceexec.SourceGuard, host string) (*Controller, error) {
	if pool == nil || metadata == nil || guard == nil || sources == nil || host == "" || len(host) > 128 {
		return nil, ErrDenied
	}
	return &Controller{pool, metadata, guard, sources, host}, nil
}

// Guard owns the exact selected proof. Guard's authority/source ordering is
// established by resourceexec before this callback; it borrows no connection.
func Guard(host string) resourceexec.SingleTabGuard {
	return func(ctx context.Context, tx pgx.Tx, b resources.Binding, id string) (resourceexec.SingleTabSession, error) {
		return readSelected(ctx, tx, b, id, host)
	}
}
