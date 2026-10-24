package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresLeaseManager struct {
	pool           *pgxpool.Pool
	authorityReads *pgxpool.Pool
	authority      *ownership.Authority
}

func NewPostgresLeaseManager(pool *pgxpool.Pool) *PostgresLeaseManager {
	return NewPostgresLeaseManagerWithAuthorityReads(pool, pool)
}

func NewPostgresLeaseManagerWithAuthorityReads(pool, reads *pgxpool.Pool) *PostgresLeaseManager {
	return &PostgresLeaseManager{pool: pool, authorityReads: reads, authority: ownership.NewAuthority(pool)}
}
func (m *PostgresLeaseManager) Authority() *ownership.Authority { return m.authority }

func (m *PostgresLeaseManager) AcquireLease(ctx context.Context, p AcquireLeaseParams) (LeaseRecord, error) {
	if err := validateAcquireLeaseParams(p); err != nil {
		return LeaseRecord{}, err
	}
	release, err := m.authority.BeginMutation(ctx)
	if err != nil {
		return LeaseRecord{}, err
	}
	defer release()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return LeaseRecord{}, err
	}
	id := "lease-" + hex.EncodeToString(raw[:])
	r, err := scanDurableLease(m.pool.QueryRow(ctx, `INSERT INTO session_ownership(session_key,lease_id,run_id,owner_id,fence_epoch,acquired_at,expires_at)
 VALUES($1,$2,$3,$4,1,clock_timestamp(),clock_timestamp()+$5*INTERVAL '1 millisecond')
 ON CONFLICT(session_key) DO UPDATE SET lease_id=EXCLUDED.lease_id,run_id=EXCLUDED.run_id,owner_id=EXCLUDED.owner_id,
 fence_epoch=session_ownership.fence_epoch+1,acquired_at=clock_timestamp(),expires_at=EXCLUDED.expires_at
 WHERE session_ownership.expires_at<=clock_timestamp() OR session_ownership.lease_id=''
 RETURNING lease_id,session_key,run_id,owner_id,fence_epoch,acquired_at,expires_at`, p.SessionKey, id, p.RunID, p.OwnerID, p.TTL.Milliseconds()))
	if errors.Is(err, pgx.ErrNoRows) {
		return LeaseRecord{}, ErrLeaseConflict
	}
	if err != nil {
		return LeaseRecord{}, fmt.Errorf("acquire durable lease: %w", err)
	}
	return r, nil
}
func (m *PostgresLeaseManager) RenewLease(ctx context.Context, id string, ttl time.Duration) (LeaseRecord, error) {
	if id == "" || ttl <= 0 {
		return LeaseRecord{}, fmt.Errorf("lease_id and positive ttl required")
	}
	release, err := m.authority.BeginMutation(ctx)
	if err != nil {
		return LeaseRecord{}, err
	}
	defer release()
	r, err := scanDurableLease(m.pool.QueryRow(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+$2*INTERVAL '1 millisecond' WHERE lease_id=$1 AND expires_at>clock_timestamp() RETURNING lease_id,session_key,run_id,owner_id,fence_epoch,acquired_at,expires_at`, id, ttl.Milliseconds()))
	if errors.Is(err, pgx.ErrNoRows) {
		return LeaseRecord{}, ErrLeaseNotFound
	}
	return r, err
}
func (m *PostgresLeaseManager) ReleaseLease(ctx context.Context, id string) (bool, error) {
	release, err := m.authority.BeginMutation(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	tag, err := m.pool.Exec(ctx, `UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp() WHERE lease_id=$1`, id)
	return tag.RowsAffected() == 1, err
}
func (m *PostgresLeaseManager) GetLease(ctx context.Context, id string) (LeaseRecord, error) {
	r, err := scanDurableLease(m.authorityReads.QueryRow(ctx, `SELECT lease_id,session_key,run_id,owner_id,fence_epoch,acquired_at,expires_at FROM session_ownership WHERE lease_id=$1 AND expires_at>clock_timestamp()`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return LeaseRecord{}, ErrLeaseNotFound
	}
	return r, err
}
func (m *PostgresLeaseManager) ValidateLease(ctx context.Context, id, runID string, epoch int64) (LeaseRecord, error) {
	r, err := m.GetLease(ctx, id)
	if err != nil {
		return r, err
	}
	if r.RunID != runID || r.FenceEpoch != epoch {
		return LeaseRecord{}, ownership.ErrStale
	}
	return r, nil
}
func scanDurableLease(row pgx.Row) (LeaseRecord, error) {
	var r LeaseRecord
	err := row.Scan(&r.LeaseID, &r.SessionKey, &r.RunID, &r.OwnerID, &r.FenceEpoch, &r.AcquiredAt, &r.ExpiresAt)
	r.TTLSeconds = int64(r.ExpiresAt.Sub(r.AcquiredAt) / time.Second)
	return r, err
}
