package budget

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Ledger struct{ pool *pgxpool.Pool }

func NewLedger(pool *pgxpool.Pool) *Ledger { return &Ledger{pool: pool} }

type scanner interface{ Scan(...any) error }

func scanScope(row scanner) (Scope, error) {
	var s Scope
	var limits, used []byte
	err := row.Scan(&s.ID, &s.Kind, &s.OwnerID, &s.SubjectID, &limits, &used, &s.Deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return Scope{}, ErrNotFound
	}
	if err != nil {
		return Scope{}, err
	}
	if err = json.Unmarshal(limits, &s.Limits); err != nil {
		return Scope{}, err
	}
	if err = json.Unmarshal(used, &s.Used); err != nil {
		return Scope{}, err
	}
	return s, nil
}

const scopeColumns = `scope_id,kind,owner_id,subject_id,limits_json,used_json,deadline`

// Ensure creates an immutable policy once. Restart/revision/changed defaults
// cannot reset spending or silently expand an existing owner's allowance.
func (l *Ledger) Ensure(ctx context.Context, spec ScopeSpec) (Scope, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return Scope{}, err
	}
	defer tx.Rollback(ctx)
	s, err := l.EnsureTx(ctx, tx, spec)
	if err != nil {
		return Scope{}, err
	}
	return s, tx.Commit(ctx)
}

// EnsureTx never acquires another connection. It is safe inside effect/child
// admission transactions, including a pool limited to two connections.
func (l *Ledger) EnsureTx(ctx context.Context, tx pgx.Tx, spec ScopeSpec) (Scope, error) {
	if err := validateScope(spec); err != nil {
		return Scope{}, err
	}
	id := ScopeID(spec.Kind, spec.OwnerID, spec.SubjectID)
	limits, _ := json.Marshal(spec.Limits)
	zero, _ := json.Marshal(Units{})
	// Concurrent initialization can meet either the canonical primary key or
	// the (kind, owner, subject) unique index. Both identify the same immutable
	// policy; still read by canonical ID and verify every field below.
	_, err := tx.Exec(ctx, `INSERT INTO execution_budget_scopes(scope_id,kind,owner_id,subject_id,limits_json,used_json,deadline) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, id, spec.Kind, spec.OwnerID, spec.SubjectID, limits, zero, spec.Deadline)
	if err != nil {
		return Scope{}, err
	}
	s, err := scanScope(tx.QueryRow(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE scope_id=$1`, id))
	if err != nil {
		return Scope{}, err
	}
	if s.Kind != spec.Kind || s.OwnerID != spec.OwnerID || s.SubjectID != spec.SubjectID || s.Limits != spec.Limits || !s.Deadline.Equal(spec.Deadline.Truncate(time.Microsecond)) {
		return Scope{}, ErrConflict
	}
	return s, nil
}
func (l *Ledger) Get(ctx context.Context, id string) (Scope, error) {
	return scanScope(l.pool.QueryRow(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE scope_id=$1`, id))
}

func (l *Ledger) Reserve(ctx context.Context, req Request) (Reservation, bool, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, false, err
	}
	defer tx.Rollback(ctx)
	r, fresh, err := l.ReserveTx(ctx, tx, req)
	if err != nil {
		return Reservation{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Reservation{}, false, err
	}
	return r, fresh, nil
}

// ReserveTx owns only budget tables. The caller may compose it with a trusted
// effect claim or child admission transaction after taking its domain locks.
func (l *Ledger) ReserveTx(ctx context.Context, tx pgx.Tx, req Request) (Reservation, bool, error) {
	req, err := canonicalRequest(req)
	if err != nil {
		return Reservation{}, false, err
	}
	// An owned savepoint also removes the placeholder if admission fails. An
	// outer transaction cannot accidentally persist a partially charged quote.
	nested, err := tx.Begin(ctx)
	if err != nil {
		return Reservation{}, false, err
	}
	defer nested.Rollback(ctx)
	r, fresh, err := l.reserve(ctx, nested, req)
	if err != nil {
		return Reservation{}, false, err
	}
	if err = nested.Commit(ctx); err != nil {
		return Reservation{}, false, err
	}
	return r, fresh, nil
}

func (l *Ledger) reserve(ctx context.Context, tx pgx.Tx, req Request) (Reservation, bool, error) {
	id := digest([]string{req.OwnerID, req.OperationID})
	fingerprint := digest(req)
	data, _ := json.Marshal(req)
	tag, err := tx.Exec(ctx, `INSERT INTO execution_budget_reservations(reservation_id,owner_id,operation_id,fingerprint,request_json) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, id, req.OwnerID, req.OperationID, fingerprint, data)
	if err != nil {
		return Reservation{}, false, err
	}
	var storedFingerprint string
	var created time.Time
	if err = tx.QueryRow(ctx, `SELECT fingerprint,created_at FROM execution_budget_reservations WHERE reservation_id=$1 FOR UPDATE`, id).Scan(&storedFingerprint, &created); err != nil {
		return Reservation{}, false, err
	}
	if fingerprint != storedFingerprint {
		return Reservation{}, false, ErrConflict
	}
	r := Reservation{ID: id, Request: req, CreatedAt: created}
	if tag.RowsAffected() == 0 {
		return r, false, nil
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Reservation{}, false, err
	}
	// All callers lock in the same sorted order. Every limit is checked before
	// any commit; a failed scope rolls the complete reservation back.
	for _, scopeID := range req.ScopeIDs {
		s, err := scanScope(tx.QueryRow(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE scope_id=$1 FOR UPDATE`, scopeID))
		if err != nil {
			return Reservation{}, false, err
		}
		if s.OwnerID != req.OwnerID {
			return Reservation{}, false, ErrConflict
		}
		next, err := addWithin(s, req.Charge, now)
		if err != nil {
			return Reservation{}, false, err
		}
		used, _ := json.Marshal(next)
		if _, err = tx.Exec(ctx, `UPDATE execution_budget_scopes SET used_json=$2,updated_at=clock_timestamp() WHERE scope_id=$1`, scopeID, used); err != nil {
			return Reservation{}, false, err
		}
	}
	return r, true, nil
}
