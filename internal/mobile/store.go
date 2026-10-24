package mobile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool     *pgxpool.Pool
	Catalog  *catalog.Store
	OTPRelay interface {
		RelaySMS(context.Context, pgx.Tx, string, string, credentials.SMSCandidate) (bool, error)
	}
}

func (s *Store) RelayLoginSMS(ctx context.Context, id, token string, c credentials.SMSCandidate) (bool, error) {
	if s.OTPRelay == nil {
		return false, ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, ErrDenied
	}
	defer tx.Rollback(ctx)
	owner, status, err := deviceTx(ctx, tx, id, token)
	if err != nil || status != "ACTIVE" {
		return false, ErrDenied
	}
	var count int
	if tx.QueryRow(ctx, `INSERT INTO mobile_request_limits(key_hash) VALUES($1) ON CONFLICT(key_hash) DO UPDATE SET request_count=CASE WHEN mobile_request_limits.window_start<now()-interval '1 minute' THEN 1 ELSE mobile_request_limits.request_count+1 END,window_start=CASE WHEN mobile_request_limits.window_start<now()-interval '1 minute' THEN now() ELSE mobile_request_limits.window_start END RETURNING request_count`, digest([]byte("otp-relay:"+id))).Scan(&count) != nil {
		return false, ErrDenied
	}
	if count > 60 {
		return false, ErrLimit
	}
	accepted, err := s.OTPRelay.RelaySMS(ctx, tx, owner, id, c)
	if err != nil {
		return false, ErrInvalid
	}
	if tx.Commit(ctx) != nil {
		return false, ErrDenied
	}
	return accepted, nil
}

func random() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func owner(ctx context.Context) (string, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return "", ErrDenied
	}
	return a.OwnerID, nil
}

// Limits are shared across replicas. The cardinality is capped: callers use a
// global pairing key, not arbitrary client-controlled identities.
func (s *Store) PairLimit(ctx context.Context) error {
	var n int
	err := s.Pool.QueryRow(ctx, `INSERT INTO mobile_request_limits(key_hash) VALUES('pairing-global') ON CONFLICT(key_hash) DO UPDATE SET request_count=CASE WHEN mobile_request_limits.window_start<clock_timestamp()-interval '1 minute' THEN 1 ELSE mobile_request_limits.request_count+1 END,window_start=CASE WHEN mobile_request_limits.window_start<clock_timestamp()-interval '1 minute' THEN clock_timestamp() ELSE mobile_request_limits.window_start END RETURNING request_count`).Scan(&n)
	if err != nil {
		return err
	}
	if n > 60 {
		return ErrLimit
	}
	return nil
}
func (s *Store) CreateCode(ctx context.Context) (map[string]any, error) {
	own, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	code, err := random()
	if err != nil {
		return nil, err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO mobile_pairings(session_hash,code_hash,owner_id,status) VALUES($1,$1,$2,'APPROVED')`, digest([]byte(code)), own)
	return map[string]any{"pairing_code": code, "expires_in_sec": 300}, err
}
func (s *Store) RequestPair(ctx context.Context, p PairRequest) (map[string]any, error) {
	if p.Validate() != nil {
		return nil, ErrInvalid
	}
	session, err := random()
	if err != nil {
		return nil, err
	}
	raw, err := random()
	if err != nil {
		return nil, err
	}
	code := strings.ToUpper(raw[:10])
	_, err = s.Pool.Exec(ctx, `INSERT INTO mobile_pairings(session_hash,code_hash,device_id,device_name,android_sdk,app_version,status) VALUES($1,$2,$3,$4,$5,$6,'PENDING')`, digest([]byte(session)), digest([]byte(code)), p.Device, p.Name, p.SDK, p.Version)
	return map[string]any{"pairing_session_id": session, "user_verification_code": code, "expires_in_sec": 300}, err
}
func (s *Store) ApprovePair(ctx context.Context, code string) error {
	own, err := owner(ctx)
	if err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE mobile_pairings SET status='APPROVED',owner_id=$2 WHERE code_hash=$1 AND status='PENDING' AND expires_at>clock_timestamp()`, digest([]byte(strings.ToUpper(strings.TrimSpace(code)))), own)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return err
}
func (s *Store) Exchange(ctx context.Context, p PairRequest) (PairResult, error) {
	if p.Validate() != nil {
		return PairResult{}, ErrInvalid
	}
	return s.consumePair(ctx, digest([]byte(p.Code)), "", p)
}
func (s *Store) Poll(ctx context.Context, session, device string) (map[string]any, error) {
	result, err := s.consumePair(ctx, "", digest([]byte(session)), PairRequest{Device: device})
	if err == errPending {
		return map[string]any{"status": "PENDING"}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "APPROVED", "device_token": result.Token, "server_name": result.Server}, nil
}

var errPending = errors.New("pairing_pending")

func (s *Store) consumePair(ctx context.Context, code, session string, p PairRequest) (PairResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PairResult{}, err
	}
	defer tx.Rollback(ctx)
	var own, status, device, name, version, sh string
	var sdk int
	err = tx.QueryRow(ctx, `SELECT session_hash,COALESCE(owner_id,''),status,device_id,device_name,android_sdk,app_version FROM mobile_pairings WHERE (($1<>'' AND code_hash=$1 AND device_id='') OR ($2<>'' AND session_hash=$2 AND device_id=$3)) AND expires_at>clock_timestamp() FOR UPDATE`, code, session, p.Device).Scan(&sh, &own, &status, &device, &name, &sdk, &version)
	if err != nil {
		return PairResult{}, ErrDenied
	}
	if status == "PENDING" {
		return PairResult{}, errPending
	}
	if status != "APPROVED" || own == "" {
		return PairResult{}, ErrDenied
	}
	if session != "" {
		p.Name = name
		p.SDK = sdk
		p.Version = version
	}
	token, err := random()
	if err != nil {
		return PairResult{}, err
	}
	// Reusing a device ID is never an implicit transfer, rotation or takeover.
	tag, err := tx.Exec(ctx, `INSERT INTO mobile_devices(device_id,owner_id,device_name,token_hash,android_sdk,app_version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(device_id) DO NOTHING`, p.Device, own, p.Name, digest([]byte(token)), p.SDK, p.Version)
	if err != nil {
		return PairResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return PairResult{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE mobile_pairings SET status='CONSUMED' WHERE session_hash=$1`, sh); err != nil {
		return PairResult{}, err
	}
	result := PairResult{Device: p.Device, Token: token, Server: "Butler", Paired: time.Now().UTC()}
	return result, tx.Commit(ctx)
}

type Device struct {
	ID        string     `json:"device_id"`
	Name      string     `json:"device_name"`
	Status    string     `json:"status"`
	SDK       int        `json:"android_sdk"`
	Version   string     `json:"app_version"`
	LastSeen  *time.Time `json:"last_seen_at"`
	Accepted  int64      `json:"accepted_count"`
	Duplicate int64      `json:"duplicate_count"`
	Rejected  int64      `json:"rejected_count"`
	Retention int        `json:"retention_days"`
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	own, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT device_id,device_name,status,android_sdk,app_version,last_seen_at,accepted_count,duplicate_count,rejected_count,retention_days FROM mobile_devices WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 100`, own)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err = rows.Scan(&d.ID, &d.Name, &d.Status, &d.SDK, &d.Version, &d.LastSeen, &d.Accepted, &d.Duplicate, &d.Rejected, &d.Retention); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func deviceTx(ctx context.Context, tx pgx.Tx, id, token string) (string, string, error) {
	var own, status string
	if len(token) != 64 || !identifier.MatchString(id) {
		return "", "", ErrDenied
	}
	err := tx.QueryRow(ctx, `SELECT owner_id,status FROM mobile_devices WHERE device_id=$1 AND token_hash=$2 FOR UPDATE`, id, digest([]byte(token))).Scan(&own, &status)
	if err != nil || status == "REVOKED" {
		return "", "", ErrDenied
	}
	return own, status, nil
}
func (s *Store) Status(ctx context.Context, id, token string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	_, status, err := deviceTx(ctx, tx, id, token)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE mobile_devices SET last_seen_at=clock_timestamp() WHERE device_id=$1`, id)
	if err != nil {
		return "", err
	}
	return status, tx.Commit(ctx)
}
func (s *Store) ChangeDevice(ctx context.Context, id, status string, days int) error {
	own, err := owner(ctx)
	if err != nil {
		return err
	}
	if (status != "ACTIVE" && status != "PAUSED" && status != "REVOKED") || days < 1 || days > 365 {
		return ErrInvalid
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE mobile_devices SET status=$3,retention_days=$4 WHERE device_id=$1 AND owner_id=$2 AND (status<>'REVOKED' OR $3='REVOKED')`, id, own, status, days)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return err
}
func (s *Store) Disconnect(ctx context.Context, id, token string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, _, err = deviceTx(ctx, tx, id, token); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE mobile_devices SET status='REVOKED' WHERE device_id=$1`, id)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Rotate(ctx context.Context, id, token string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, _, err = deviceTx(ctx, tx, id, token); err != nil {
		return "", err
	}
	next, err := random()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE mobile_devices SET token_hash=$2 WHERE device_id=$1`, id, digest([]byte(next)))
	if err != nil {
		return "", err
	}
	return next, tx.Commit(ctx)
}
func (s *Store) Accept(ctx context.Context, id, token string, b Batch) (ACK, error) {
	ack := ACK{Accepted: []string{}, Duplicate: []string{}, Conflict: []string{}, Rejected: []Rejected{}, Time: time.Now().UnixMilli()}
	if b.Validate(time.Now()) != nil {
		return ack, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ack, err
	}
	defer tx.Rollback(ctx)
	own, status, err := deviceTx(ctx, tx, id, token)
	if err != nil {
		return ack, err
	}
	if status != "ACTIVE" {
		return ack, ErrPaused
	}
	var n int
	err = tx.QueryRow(ctx, `UPDATE mobile_devices SET request_count=CASE WHEN request_window<clock_timestamp()-interval '1 minute' THEN 1 ELSE request_count+1 END, request_window=CASE WHEN request_window<clock_timestamp()-interval '1 minute' THEN clock_timestamp() ELSE request_window END,last_seen_at=clock_timestamp() WHERE device_id=$1 RETURNING request_count`, id).Scan(&n)
	if err != nil {
		return ack, err
	}
	if n > 120 {
		if err = tx.Commit(ctx); err != nil {
			return ack, err
		}
		return ack, ErrLimit
	}
	if err = s.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return ack, err
	}
	encodedBatch, _ := json.Marshal(b)
	batchHash := digest(encodedBatch)
	var oldBatchHash string
	err = tx.QueryRow(ctx, `SELECT body_hash FROM mobile_batches WHERE device_id=$1 AND batch_id=$2`, id, b.ID).Scan(&oldBatchHash)
	if err == nil && oldBatchHash != batchHash {
		return ack, ErrConflict
	}
	if err != nil && err != pgx.ErrNoRows {
		return ack, err
	}
	for _, event := range b.Events {
		raw, _ := json.Marshal(event)
		hash := digest(raw)
		var previous string
		err = tx.QueryRow(ctx, `SELECT event_hash FROM mobile_events WHERE device_id=$1 AND device_event_id=$2`, id, event.ID).Scan(&previous)
		if err == nil {
			if previous == hash {
				ack.Duplicate = append(ack.Duplicate, event.ID)
			} else {
				ack.Conflict = append(ack.Conflict, event.ID)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return ack, err
		}
		sid, e := s.Catalog.PutMobileObservationTx(ctx, tx, own, id+":"+event.ID, string(raw), event.Package, time.UnixMilli(event.Observed))
		if errors.Is(e, catalog.ErrDeleted) {
			ack.Rejected = append(ack.Rejected, Rejected{ID: event.ID, Code: "SOURCE_DELETED", Reason: "Source was deleted"})
			continue
		}
		if e != nil {
			return ack, e
		}
		_, err = tx.Exec(ctx, `INSERT INTO mobile_events(device_id,device_event_id,event_hash,notification_hash,package_name,previous_device_event_id,change_type,observed_at,posted_at,source_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, event.ID, hash, digest([]byte(event.Key)), event.Package, event.Previous, event.Change, time.UnixMilli(event.Observed), time.UnixMilli(event.Posted), sid)
		if err != nil {
			return ack, err
		}
		if err = IndexMessagesTx(ctx, tx, own, id, sid, event); err != nil {
			return ack, err
		}
		ack.Accepted = append(ack.Accepted, event.ID)
	}
	_, err = tx.Exec(ctx, `INSERT INTO mobile_batches(device_id,batch_id,body_hash) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, b.ID, batchHash)
	if err != nil {
		return ack, err
	}
	_, err = tx.Exec(ctx, `UPDATE mobile_devices SET accepted_count=accepted_count+$2,duplicate_count=duplicate_count+$3,rejected_count=rejected_count+$4 WHERE device_id=$1`, id, len(ack.Accepted), len(ack.Duplicate), len(ack.Conflict)+len(ack.Rejected))
	if err != nil {
		return ack, err
	}
	return ack, tx.Commit(ctx)
}

type Observation struct {
	ID       string    `json:"device_event_id"`
	Device   string    `json:"device_id"`
	Package  string    `json:"package_name"`
	Change   string    `json:"change_type"`
	Source   string    `json:"source_id"`
	Revision int64     `json:"revision"`
	Status   string    `json:"status"`
	Observed time.Time `json:"observed_at"`
	Received time.Time `json:"received_at"`
	Body     string    `json:"body,omitempty"`
}

func (s *Store) Events(ctx context.Context, device, query string, reveal bool) ([]Observation, error) {
	own, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	if len(query) > 256 {
		return nil, ErrInvalid
	}
	if err = s.Catalog.CheckRestore(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT e.device_event_id,e.device_id,e.package_name,e.change_type,e.source_id,s.head_revision,s.status,e.observed_at,e.received_at,CASE WHEN $4 AND s.status NOT IN ('deleted','revoked') THEN r.body ELSE '' END FROM mobile_events e JOIN mobile_devices d USING(device_id) JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE d.owner_id=$1 AND ($2='' OR e.device_id=$2) AND ($3='' OR (s.status NOT IN ('deleted','revoked') AND (strpos(lower(r.body),lower($3))>0 OR strpos(lower(e.package_name),lower($3))>0))) ORDER BY e.received_at DESC LIMIT 100`, own, device, query, reveal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Observation{}
	for rows.Next() {
		var o Observation
		if err = rows.Scan(&o.ID, &o.Device, &o.Package, &o.Change, &o.Source, &o.Revision, &o.Status, &o.Observed, &o.Received, &o.Body); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Store) DeleteEvent(ctx context.Context, source string, revision int64) error {
	own, err := owner(ctx)
	if err != nil {
		return err
	}
	var found bool
	err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mobile_events e JOIN mobile_devices d USING(device_id) WHERE e.source_id=$1 AND d.owner_id=$2)`, source, own).Scan(&found)
	if err != nil {
		return err
	}
	if !found {
		return ErrDenied
	}
	_, err = s.Catalog.Delete(ctx, source, "delete", revision)
	return err
}

// Cleanup is deterministic owner-configured retention, not model work. It uses
// the existing fsynced deletion anchor and keeps immutable delivery tombstones.
func (s *Store) Cleanup(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT e.source_id,s.head_revision FROM mobile_events e JOIN mobile_devices d USING(device_id) JOIN memory_sources s USING(source_id) WHERE s.status NOT IN ('deleted','revoked') AND e.received_at<clock_timestamp()-make_interval(days=>d.retention_days) ORDER BY e.received_at LIMIT 100`)
	if err != nil {
		return err
	}
	type item struct {
		id  string
		rev int64
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.rev); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		if _, err = s.Catalog.DeleteExpiredMobileObservation(ctx, v.id, v.rev); err != nil {
			return err
		}
	}
	_, err = s.Pool.Exec(ctx, `DELETE FROM mobile_pairings WHERE expires_at<clock_timestamp()-interval '1 hour'`)
	return err
}
