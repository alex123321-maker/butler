package initiative

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	// RunDataGuardTx is installed by the source lifecycle owner at bootstrap.
	// It uses the same transaction as promotion, with no nested pool borrowing.
	RunDataGuardTx func(context.Context, pgx.Tx, string) error
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

type ConfigReceipt struct {
	Kind      string `json:"kind"`
	SubjectID string `json:"subject_id"`
	Revision  int64  `json:"revision"`
}

func configActor(ctx context.Context) (owneridentity.Actor, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || len(a.DeliveryID) > 512 {
		return a, ErrInvalid
	}
	return a, nil
}

func (s *Store) beginConfig(ctx context.Context, a owneridentity.Actor, requestHash string) (pgx.Tx, *ConfigReceipt, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, hash([]string{a.OwnerID, a.Channel, a.DeliveryID})); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	var r ConfigReceipt
	var previousHash string
	err = tx.QueryRow(ctx, `SELECT subject_kind,subject_id,revision,request_hash FROM initiative_config_events WHERE owner_id=$1 AND channel=$2 AND delivery_id=$3`, a.OwnerID, a.Channel, a.DeliveryID).Scan(&r.Kind, &r.SubjectID, &r.Revision, &previousHash)
	if err == nil {
		if previousHash != requestHash {
			_ = tx.Rollback(ctx)
			return nil, nil, ErrConflict
		}
		return tx, &r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, nil, nil
}

func saveConfig(ctx context.Context, tx pgx.Tx, a owneridentity.Actor, r ConfigReceipt, requestHash string) error {
	_, err := tx.Exec(ctx, `INSERT INTO initiative_config_events(owner_id,channel,delivery_id,request_hash,subject_id,subject_kind,revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, a.OwnerID, a.Channel, a.DeliveryID, requestHash, r.SubjectID, r.Kind, r.Revision)
	return err
}

// SaveSource is an authenticated operator configuration boundary. Observations
// and model arguments cannot create or alter these sources or task templates.
func (s *Store) SaveSource(ctx context.Context, source Source, expectedRevision int64) (ConfigReceipt, error) {
	a, err := configActor(ctx)
	if err != nil {
		return ConfigReceipt{}, err
	}
	if expectedRevision < 0 || (source.OwnerID != "" && source.OwnerID != a.OwnerID) || (a.SessionKey != "" && a.SessionKey != source.SessionKey) {
		return ConfigReceipt{}, ErrInvalid
	}
	source.OwnerID, source.Revision = a.OwnerID, 0
	if source.Validate() != nil {
		return ConfigReceipt{}, ErrInvalid
	}
	requestHash := hash(struct {
		Source           Source
		ExpectedRevision int64
	}{source, expectedRevision})
	tx, previous, err := s.beginConfig(ctx, a, requestHash)
	if err != nil {
		return ConfigReceipt{}, err
	}
	defer tx.Rollback(ctx)
	if previous != nil {
		return *previous, tx.Commit(ctx)
	}
	var owner, channel string
	if err = tx.QueryRow(ctx, `SELECT user_id,channel FROM sessions WHERE session_key=$1 FOR SHARE`, source.SessionKey).Scan(&owner, &channel); err != nil || owner != a.OwnerID || (source.DigestChannel != "" && source.DigestChannel != channel) {
		return ConfigReceipt{}, ErrInvalid
	}
	source.Revision = expectedRevision + 1
	encoded, _ := json.Marshal(source)
	var revision int64
	if expectedRevision == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO initiative_sources(source_id,owner_id,goal_id,mandate_id,session_key,source_json,enabled) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7) ON CONFLICT(source_id) DO NOTHING RETURNING revision`, source.ID, source.OwnerID, source.GoalID, source.MandateID, source.SessionKey, encoded, source.Enabled).Scan(&revision)
	} else {
		err = tx.QueryRow(ctx, `UPDATE initiative_sources SET goal_id=$3,mandate_id=$4,session_key=$5,source_json=$6::jsonb,enabled=$7,revision=revision+1,updated_at=clock_timestamp() WHERE source_id=$1 AND owner_id=$2 AND revision=$8 RETURNING revision`, source.ID, source.OwnerID, source.GoalID, source.MandateID, source.SessionKey, encoded, source.Enabled, expectedRevision).Scan(&revision)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfigReceipt{}, ErrConflict
	}
	if err != nil {
		return ConfigReceipt{}, err
	}
	r := ConfigReceipt{Kind: "source", SubjectID: source.ID, Revision: revision}
	if err = saveConfig(ctx, tx, a, r, requestHash); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

type sourceRow interface{ Scan(...any) error }

func scanSource(row sourceRow) (Source, error) {
	var source Source
	var encoded []byte
	var revision int64
	var enabled bool
	err := row.Scan(&encoded, &revision, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return source, ErrNotFound
	}
	if err != nil {
		return source, err
	}
	if err = json.Unmarshal(encoded, &source); err != nil {
		return source, err
	}
	source.Revision, source.Enabled = revision, enabled
	return source, source.Validate()
}

func (s *Store) GetSource(ctx context.Context, id, owner string) (Source, error) {
	return scanSource(s.pool.QueryRow(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE source_id=$1 AND owner_id=$2`, id, owner))
}

func (s *Store) SaveSchedule(ctx context.Context, schedule Schedule, expectedRevision int64, now time.Time) (ConfigReceipt, error) {
	a, err := configActor(ctx)
	if err != nil || schedule.Validate() != nil || expectedRevision < 0 || now.IsZero() {
		return ConfigReceipt{}, ErrInvalid
	}
	schedule.NextAt, schedule.Revision = time.Time{}, 0
	requestHash := hash(struct {
		Schedule         Schedule
		ExpectedRevision int64
	}{schedule, expectedRevision})
	tx, previous, err := s.beginConfig(ctx, a, requestHash)
	if err != nil {
		return ConfigReceipt{}, err
	}
	defer tx.Rollback(ctx)
	if previous != nil {
		return *previous, tx.Commit(ctx)
	}
	if _, err = scanSource(tx.QueryRow(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE source_id=$1 AND owner_id=$2 FOR SHARE`, schedule.SourceID, a.OwnerID)); err != nil {
		return ConfigReceipt{}, err
	}
	schedule.Revision = expectedRevision + 1
	schedule.NextAt = schedule.NextAfter(now)
	if schedule.NextAt.IsZero() {
		return ConfigReceipt{}, ErrInvalid
	}
	encoded, _ := json.Marshal(schedule)
	var revision int64
	if expectedRevision == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO initiative_schedules(schedule_id,source_id,schedule_json,next_at,enabled) VALUES($1,$2,$3::jsonb,$4,$5) ON CONFLICT(schedule_id) DO NOTHING RETURNING revision`, schedule.ID, schedule.SourceID, encoded, schedule.NextAt, schedule.Enabled).Scan(&revision)
	} else {
		err = tx.QueryRow(ctx, `UPDATE initiative_schedules SET schedule_json=$3::jsonb,next_at=$4,enabled=$5,revision=revision+1,updated_at=clock_timestamp() WHERE schedule_id=$1 AND source_id=$2 AND revision=$6 RETURNING revision`, schedule.ID, schedule.SourceID, encoded, schedule.NextAt, schedule.Enabled, expectedRevision).Scan(&revision)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfigReceipt{}, ErrConflict
	}
	if err != nil {
		return ConfigReceipt{}, err
	}
	r := ConfigReceipt{Kind: "schedule", SubjectID: schedule.ID, Revision: revision}
	if err = saveConfig(ctx, tx, a, r, requestHash); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
