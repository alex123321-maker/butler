package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// PutMobileObservationTx is a trusted ingress seam, called only after device
// authentication under the device row lock. Each immutable event is a separate
// private DATA revision. Owner-configured mobile automation activates DATA reads
// without per-message review; absent that standing policy it is quarantined.
// Receipts and sources commit in the caller's single transaction.
func (s *Store) PutMobileObservationTx(ctx context.Context, tx pgx.Tx, own, key, body, author string, at time.Time) (string, error) {
	if own == "" || len(key) > 512 || len(body) > 128*1024 || !json.Valid([]byte(body)) {
		return "", ErrInvalid
	}
	if err := s.CheckRestoreTx(ctx, tx); err != nil {
		return "", err
	}
	id := sourceID(own, "mobile:"+key)
	var deleted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_deletions WHERE source_id=$1)`, id).Scan(&deleted); err != nil {
		return "", err
	}
	if deleted {
		return "", ErrDeleted
	}
	origin := content.Origin{Kind: content.Document, SourceRefs: []string{id}, Sensitivity: content.Private}
	if sanitize.Text(body) != body {
		origin.Sensitivity = content.Secret
	}
	status := "quarantined"
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mobile_agent_access WHERE owner_id=$1 AND enabled)`, own).Scan(&enabled); err != nil {
		return "", err
	}
	if enabled {
		status = "active"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key,status) VALUES($1,$2,'document','user',$2,$3,$4)`, id, own, "mobile:"+key, status); err != nil {
		return "", err
	}
	_, err := tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time) VALUES($1,1,$2,'Android external observation',$3,$4::jsonb,$5::jsonb,$6,$7)`, id, hash(body), body, `{"adapter":"android_notification_v1"}`, mustJSON(origin), author, at)
	return id, err
}
