package generatedtools

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

type ControlRequest struct {
	Operation  string `json:"operation"`
	Digest     string `json:"digest,omitempty"`
	Generation int64  `json:"generation,omitempty"`
}
type VersionEvidence struct {
	Record       Record        `json:"record"`
	Checks       *Report       `json:"checks,omitempty"`
	Trials       int64         `json:"trials"`
	PassedTrials int64         `json:"passed_trials"`
	Uses         int64         `json:"uses"`
	Outputs      []UseEvidence `json:"outputs"`
}
type UseEvidence struct {
	RunID        string            `json:"run_id"`
	TaskID       string            `json:"task_id"`
	TaskRevision int64             `json:"task_revision"`
	Generation   int64             `json:"generation"`
	Source       catalog.SourceRef `json:"source_ref"`
	SHA256       string            `json:"sha256"`
}
type ControlResponse struct {
	Items []VersionEvidence `json:"items,omitempty"`
	Item  *Record           `json:"item,omitempty"`
	Error string            `json:"error,omitempty"`
}

func (s *Store) actor(ctx context.Context) (owneridentity.Actor, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.pool == nil || a.OwnerID != s.owner {
		return a, ErrAccess
	}
	return a, nil
}
func (s *Store) List(ctx context.Context) ([]VersionEvidence, error) {
	if _, e := s.actor(ctx); e != nil {
		return nil, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT digest FROM converter_versions WHERE owner_id=$1 ORDER BY converter_id,created_at DESC,digest LIMIT 64`, s.owner)
	if e != nil {
		return nil, e
	}
	hashes := []string{}
	for rows.Next() {
		var h string
		if e = rows.Scan(&h); e != nil {
			rows.Close()
			return nil, e
		}
		hashes = append(hashes, h)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	result := []VersionEvidence{}
	for _, h := range hashes {
		r, e := s.metadataTx(ctx, tx, h)
		if e != nil {
			return nil, e
		}
		v := VersionEvidence{Record: r, Outputs: []UseEvidence{}}
		if e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='passed') FROM converter_trials WHERE owner_id=$1 AND digest=$2`, s.owner, h).Scan(&v.Trials, &v.PassedTrials); e != nil {
			return nil, e
		}
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM converter_usages WHERE owner_id=$1 AND digest=$2`, s.owner, h).Scan(&v.Uses); e != nil {
			return nil, e
		}
		rows, e := tx.Query(ctx, `SELECT run_id,task_id,task_revision,generation,output_ref_json,output_hash FROM converter_usages WHERE owner_id=$1 AND digest=$2 ORDER BY recorded_at DESC LIMIT 4`, s.owner, h)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var use UseEvidence
			var raw []byte
			if e = rows.Scan(&use.RunID, &use.TaskID, &use.TaskRevision, &use.Generation, &raw, &use.SHA256); e != nil || json.Unmarshal(raw, &use.Source) != nil || !digestName.MatchString(use.SHA256) {
				rows.Close()
				return nil, ErrAccess
			}
			v.Outputs = append(v.Outputs, use)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		var raw []byte
		if tx.QueryRow(ctx, `SELECT report_json FROM converter_validations WHERE owner_id=$1 AND digest=$2 ORDER BY validation_id DESC LIMIT 1`, s.owner, h).Scan(&raw) == nil {
			var report Report
			if Decode(raw, &report) != nil {
				return nil, ErrChecks
			}
			v.Checks = &report
		}
		result = append(result, v)
	}
	return result, tx.Commit(ctx)
}
func (s *Store) Revoke(ctx context.Context, hash string, generation int64) (Record, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return Record{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer tx.Rollback(ctx)
	r, e := s.metadataTx(ctx, tx, hash)
	if e != nil {
		return r, e
	}
	var current int64
	var active string
	if tx.QueryRow(ctx, `SELECT generation,COALESCE(active_digest,'') FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR UPDATE`, s.owner, r.ID).Scan(&current, &active) != nil || current != generation {
		return r, ErrAccess
	}
	var status string
	if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR UPDATE`, s.owner, hash).Scan(&status) != nil || status == "revoked" {
		return r, ErrAccess
	}
	if _, e = tx.Exec(ctx, `UPDATE converter_states SET status='revoked',updated_at=clock_timestamp() WHERE owner_id=$1 AND digest=$2`, s.owner, hash); e != nil {
		return r, e
	}
	if _, e = tx.Exec(ctx, `UPDATE converter_catalog SET generation=generation+1,active_digest=CASE WHEN active_digest=$3 THEN NULL ELSE active_digest END,updated_at=clock_timestamp() WHERE owner_id=$1 AND converter_id=$2`, s.owner, r.ID, hash); e != nil {
		return r, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_events(owner_id,converter_id,digest,generation,kind,source_channel,delivery_id) VALUES($1,$2,$3,$4,'owner_revoke',$5,$6)`, s.owner, r.ID, hash, current+1, a.Channel, a.DeliveryID); e != nil {
		return r, ErrAccess
	}
	r, e = s.metadataTx(ctx, tx, hash)
	if e != nil {
		return r, e
	}
	return r, tx.Commit(ctx)
}
func (s *Store) Control(ctx context.Context, r ControlRequest) (ControlResponse, error) {
	result := ControlResponse{}
	if _, e := s.actor(ctx); e != nil {
		return result, e
	}
	var e error
	switch r.Operation {
	case "list":
		if r.Digest != "" || r.Generation != 0 {
			e = ErrPackage
		} else {
			result.Items, e = s.List(ctx)
		}
	case "revoke":
		var item Record
		item, e = s.Revoke(ctx, r.Digest, r.Generation)
		result.Item = &item
	default:
		e = ErrPackage
	}
	if e != nil {
		result.Error = SafeError(e)
	}
	return result, e
}
