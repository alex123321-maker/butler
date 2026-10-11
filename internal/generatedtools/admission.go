package generatedtools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

func normalizeDiscovery(d Discovery, n Need) (Discovery, error) {
	if len(d.Tools) > 64 || len(d.Skills) > 12 {
		return d, ErrLimit
	}
	d.Compatible = []string{}
	sort.Slice(d.Tools, func(i, j int) bool { return d.Tools[i].Name < d.Tools[j].Name })
	for i, c := range d.Tools {
		if c.Name == "" || len(c.Name) > 160 || !digestName.MatchString(c.ContractHash) || c.Revision == "" || i > 0 && d.Tools[i-1].Name == c.Name {
			return d, ErrChecks
		}
		if c.Class == Class && c.InputSchemaHash == Hash(n.Input) && c.OutputSchemaHash == Hash(n.Output) {
			d.Compatible = append(d.Compatible, c.Name)
		}
	}
	sort.Slice(d.Skills, func(i, j int) bool { return d.Skills[i].ID < d.Skills[j].ID })
	for i, k := range d.Skills {
		if !identifier.MatchString(k.ID) || !digestName.MatchString(k.Digest) || i > 0 && d.Skills[i-1].ID == k.ID {
			return d, ErrChecks
		}
	}
	return d, nil
}
func (s *Store) needTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, ref catalog.SourceRef) (Need, catalog.Source, error) {
	rows, e := s.sources.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read")
	if e != nil || len(rows) != 1 {
		return Need{}, catalog.Source{}, ErrAccess
	}
	n, e := needFrom(rows[0])
	if e != nil {
		return n, rows[0], e
	}
	return n, rows[0], nil
}
func (s *Store) recordedNeedTx(ctx context.Context, tx pgx.Tx, c actionauth.ContextSnapshot, n Need, ref catalog.SourceRef, d Discovery) error {
	var data, discovery []byte
	var hash, taskID string
	var revision, constraints int64
	e := tx.QueryRow(ctx, `SELECT need_hash,source_ref_json,task_id,task_revision,constraints_revision,discovery_json FROM converter_needs WHERE owner_id=$1 AND need_id=$2`, s.owner, n.ID).Scan(&hash, &data, &taskID, &revision, &constraints, &discovery)
	var saved catalog.SourceRef
	var snapshot Discovery
	if e != nil || json.Unmarshal(data, &saved) != nil || json.Unmarshal(discovery, &snapshot) != nil || hash != Hash(n) || saved != ref || taskID != c.TaskID || revision != c.TaskRevision || constraints != c.ConstraintsRevision || Hash(snapshot) != Hash(d) {
		return ErrAccess
	}
	return nil
}

// Discover must actually execute before development. Its durable metadata pins
// the current need/task versions and bounded tool/skill search snapshot; a model
// assertion that it searched, or an author-declared absence, is never accepted.
func (s *Store) Discover(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, ref catalog.SourceRef) (Need, Discovery, error) {
	c, e := s.run(ctx, call.GetRunId())
	if e != nil {
		return Need{}, Discovery{}, e
	}
	ctx = runContext(ctx, c)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Need{}, Discovery{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Need{}, Discovery{}, e
	}
	n, _, e := s.needTx(ctx, tx, c, ref)
	if e != nil {
		return n, Discovery{}, e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return n, Discovery{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return n, Discovery{}, e
	}
	d, e := s.environment.Discover(ctx, c.RunID, n)
	if e != nil {
		return n, d, e
	}
	d, e = normalizeDiscovery(d, n)
	if e != nil {
		return n, d, e
	}
	tx, e = s.pool.Begin(ctx)
	if e != nil {
		return n, d, e
	}
	defer tx.Rollback(ctx)
	m, now, e = s.guardRunTx(ctx, tx, c)
	if e != nil {
		return n, d, e
	}
	fresh, _, e := s.needTx(ctx, tx, c, ref)
	if e != nil || Hash(fresh) != Hash(n) {
		return n, d, ErrAccess
	}
	if _, e = s.permission(m, n, now); e != nil {
		return n, d, e
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return n, d, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_needs(owner_id,need_id,converter_id,need_hash,source_ref_json,task_id,task_revision,constraints_revision,run_id,discovery_json) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10::jsonb) ON CONFLICT DO NOTHING`, s.owner, n.ID, n.ConverterID, Hash(n), encoded(ref), c.TaskID, c.TaskRevision, c.ConstraintsRevision, c.RunID, encoded(d)); e != nil {
		return n, d, e
	}
	if e = s.recordedNeedTx(ctx, tx, c, n, ref, d); e != nil {
		return n, d, e
	}
	return n, d, tx.Commit(ctx)
}

func (s *Store) Develop(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, ref catalog.SourceRef, v string) (Record, error) {
	c, e := s.run(ctx, call.GetRunId())
	if e != nil {
		return Record{}, e
	}
	ctx = runContext(ctx, c)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Record{}, e
	}
	n, source, e := s.needTx(ctx, tx, c, ref)
	if e != nil {
		return Record{}, e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return Record{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Record{}, e
	}
	d, e := s.environment.Discover(ctx, c.RunID, n)
	if e != nil {
		return Record{}, e
	}
	d, e = normalizeDiscovery(d, n)
	if e != nil || len(d.Compatible) > 0 {
		return Record{}, ErrChecks
	}
	tx, e = s.pool.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e = s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Record{}, e
	}
	if _, _, e = s.needTx(ctx, tx, c, ref); e != nil {
		return Record{}, e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return Record{}, e
	}
	if e = s.recordedNeedTx(ctx, tx, c, n, ref, d); e != nil {
		return Record{}, fmt.Errorf("discovery binding: %w", e)
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return Record{}, fmt.Errorf("developer dispatch: %w", e)
	}
	if e = tx.Commit(ctx); e != nil {
		return Record{}, e
	}
	built, e := s.runtime.Develop(ctx, &converterv1.DevelopRequest{Proof: proof(ctx, call), NeedSource: dataEnvelope(source), NeedHash: Hash(n), ProfilePin: Hash(s.profile), Version: v})
	if e != nil {
		return Record{}, ErrChecks
	}
	p, e := Parse(built.GetPackage())
	if e != nil || p.Digest != built.GetPackageHash() || p.Manifest.ID != n.ConverterID || p.Manifest.NeedID != n.ID || p.Manifest.Version != v || p.Manifest.Dependencies[0] != s.profile.Dependency {
		return Record{}, ErrChecks
	}
	// Publication cannot use the authority or source snapshot from before RPC.
	tx, e = s.pool.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer tx.Rollback(ctx)
	m, now, e = s.guardRunTx(ctx, tx, c)
	if e != nil {
		return Record{}, e
	}
	fresh, _, e := s.needTx(ctx, tx, c, ref)
	if e != nil || Hash(fresh) != Hash(n) {
		return Record{}, ErrAccess
	}
	if _, e = s.permission(m, n, now); e != nil {
		return Record{}, e
	}
	if e = s.recordedNeedTx(ctx, tx, c, n, ref, d); e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_catalog(owner_id,converter_id,generation) VALUES($1,$2,1) ON CONFLICT DO NOTHING`, s.owner, n.ConverterID); e != nil {
		return Record{}, e
	}
	var generation int64
	if e = tx.QueryRow(ctx, `SELECT generation FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR UPDATE`, s.owner, n.ConverterID).Scan(&generation); e != nil {
		return Record{}, e
	}
	var previous string
	if e = tx.QueryRow(ctx, `SELECT digest FROM converter_versions WHERE owner_id=$1 AND converter_id=$2 AND version=$3`, s.owner, n.ConverterID, v).Scan(&previous); e == nil {
		return Record{}, ErrChecks
	} else if e != pgx.ErrNoRows {
		return Record{}, e
	}
	if e = s.dispatchTx(ctx, tx, c, call, contract); e != nil {
		return Record{}, e
	}
	// Creating a fresh child is safe after source locks; it cannot lock an
	// existing source out of order. Raw package bytes live only in this child.
	body, e := s.sources.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read", n.ConverterID, string(p.Raw), json.RawMessage(encoded(map[string]string{"kind": "generated_converter_v1", "digest": p.Digest})))
	if e != nil {
		return Record{}, fmt.Errorf("candidate source publication: %w", e)
	}
	permission, e := s.permission(m, n, now)
	if e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_versions(owner_id,digest,converter_id,version,need_id,need_hash,source_ref_json,profile_pin,created_by_run,permission_shape_json) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10::jsonb) ON CONFLICT DO NOTHING`, s.owner, p.Digest, n.ConverterID, v, n.ID, Hash(n), encoded(sourceRef(body)), Hash(s.profile), c.RunID, encoded(permission)); e != nil {
		return Record{}, e
	}
	var saved string
	if tx.QueryRow(ctx, `SELECT digest FROM converter_versions WHERE owner_id=$1 AND converter_id=$2 AND version=$3`, s.owner, n.ConverterID, v).Scan(&saved) != nil || saved != p.Digest {
		return Record{}, ErrChecks
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_states(owner_id,digest,status) VALUES($1,$2,'candidate') ON CONFLICT DO NOTHING`, s.owner, p.Digest); e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO converter_events(owner_id,converter_id,digest,generation,kind,run_id,action_id) VALUES($1,$2,$3,$4,'candidate',$5,$6)`, s.owner, n.ConverterID, p.Digest, generation, c.RunID, actionauth.DispatchIdentityFromContext(ctx).ActionID); e != nil {
		return Record{}, e
	}
	r, e := s.metadataTx(ctx, tx, p.Digest)
	if e != nil {
		return r, fmt.Errorf("candidate metadata: %w", e)
	}
	return r, tx.Commit(ctx)
}
