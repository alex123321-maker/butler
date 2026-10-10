package extraction

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	catalog *catalog.Store
	jobs    *jobs.Store
	backend Backend
	target  budget.LocalTarget
	zone    string
}

func New(c *catalog.Store, j *jobs.Store, b Backend, t budget.LocalTarget, price budget.ModelPrice, zone string) (*Service, error) {
	if c == nil || j == nil || b == nil || t.Endpoint() != "local-models:9090" || t.Identity() == "" || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return nil, ErrUnavailable
	}
	if zone != "" {
		if _, e := time.LoadLocation(zone); e != nil {
			return nil, ErrInvalid
		}
	}
	return &Service{c, j, b, t, zone}, nil
}
func pipeline(zone string) string {
	return digest([]string{Profile, Revision, Runtime, Schema, Normalizer, zone})
}
func (s *Service) source(ctx context.Context, tx pgx.Tx, ref catalog.SourceRef) (catalog.Source, error) {
	values, e := s.catalog.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_extract")
	if e != nil {
		return catalog.Source{}, e
	}
	src := values[0]
	text := src.Body + src.Title + src.Author + string(src.Locator)
	if src.Origin.Sensitivity == content.Secret || sanitize.Text(text) != text || src.Hash != ref.Hash {
		return catalog.Source{}, catalog.ErrDenied
	}
	return src, nil
}

// Run requires an actual current extract-job claim. It cannot revive a run lease
// or accept an owner policy. Full correlated output publishes atomically; every
// actual window inference gets a fresh shared budget reservation.
func (s *Service) Run(ctx context.Context, j jobs.Job, ref catalog.SourceRef) ([]catalog.Claim, error) {
	if s == nil || j.Kind != "extract" || j.Purpose != "memory_extract" || j.ClaimID == "" || j.ClaimEpoch < 1 || ref.ID == "" || ref.Revision < 1 || ref.AccessRevision < 1 || len(ref.Hash) != 64 {
		return nil, ErrInvalid
	}
	ctx = s.jobs.Context(ctx, j)
	tx, e := s.catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	src, e := s.source(ctx, tx, ref)
	if e != nil {
		return nil, e
	}
	pid := pipeline(s.zone)
	eid := digest([]any{src.OwnerID, src.ID, src.Revision, src.Hash, pid})
	if _, e = tx.Exec(ctx, `INSERT INTO memory_extraction_runs(extraction_id,source_id,source_revision,pipeline_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, eid, src.ID, src.Revision, pid); e != nil {
		return nil, e
	}
	var complete bool
	var ids []string
	if e = tx.QueryRow(ctx, `SELECT complete,claim_ids FROM memory_extraction_runs WHERE extraction_id=$1`, eid).Scan(&complete, &ids); e != nil {
		return nil, e
	}
	if complete {
		claims, e := readClaims(ctx, tx, ids)
		if e != nil {
			return nil, e
		}
		return claims, tx.Commit(ctx)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	cat, e := s.backend.Describe(ctx, &pb.Empty{})
	if e != nil || cat == nil {
		return nil, ErrUnavailable
	}
	found := false
	for _, p := range cat.Models {
		if p != nil && p.Id == Profile && p.Model == Model && p.Revision == Revision && p.RuntimeVersion == Runtime && p.Enabled && p.PreferredDevice == "cpu" {
			found = true
		}
	}
	if !found {
		return nil, ErrUnavailable
	}
	parts, e := windows(src.Body)
	if e != nil {
		return nil, e
	}
	var all []Record
	for _, part := range parts {
		op, e := budget.NewOperationID()
		if e != nil {
			return nil, e
		}
		request := &pb.InferRequest{RequestId: op, ProfileId: Profile, CallerId: "extract:" + digest(j.ID), TimeoutMs: 60000, CpuOnly: true, Input: &pb.InferRequest_Extract{Extract: &pb.ExtractInput{Text: part.Text, SchemaVersion: Schema}}}
		raw, _ := json.Marshal(request)
		callCtx, release, e := s.jobs.Admit(ctx, j, jobs.Operation{ID: op, Kind: "extract", Endpoint: s.target.Endpoint(), ServiceIdentity: s.target.Identity(), InputBytes: int64(len(raw)), Tokens: 1024, Timeout: time.Minute, Origin: src.Origin})
		if e != nil {
			return nil, e
		}
		result, e := s.backend.Infer(callCtx, request)
		release()
		if e != nil {
			return nil, ErrUnavailable
		}
		if result == nil || result.RequestId != op || result.ProfileId != Profile || result.Revision != Revision || result.Device != "cpu" || localmodels.ValidateMemoryExtraction(part.Text, result.GetExtract()) != nil {
			return nil, ErrInvalid
		}
		all = append(all, records(src, result.GetExtract(), part.Start, s.zone)...)
		if len(all) > 1024 {
			return nil, ErrInvalid
		}
	}
	all = uniqueRecords(all)
	if len(all) > 256 {
		return nil, ErrInvalid
	}
	tx, e = s.catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.source(ctx, tx, ref); e != nil {
		return nil, e
	}
	var extracted time.Time
	if e = tx.QueryRow(ctx, `UPDATE memory_extraction_runs SET extracted_at=COALESCE(extracted_at,clock_timestamp()) WHERE extraction_id=$1 RETURNING extracted_at`, eid).Scan(&extracted); e != nil {
		return nil, e
	}
	setTime(all, extracted)
	var claims []catalog.Claim
	// pgx encodes a nil slice as SQL NULL. A valid empty extraction is a
	// completed publication with an empty array, and must be cacheable.
	ids = []string{}
	for _, r := range all {
		value, e := json.Marshal(r)
		if e != nil {
			return nil, ErrInvalid
		}
		key := recordKey(r)
		claim, e := s.catalog.ProposeTx(ctx, tx, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "extracted:" + r.Kind + ":" + key, Value: value, Summary: r.Quote, Confidence: r.Score, DedupKey: digest([]string{pid, key})})
		if e != nil {
			return nil, e
		}
		claims = append(claims, claim)
		ids = append(ids, claim.ID)
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_extraction_runs SET complete=TRUE,claim_ids=$2 WHERE extraction_id=$1`, eid, ids); e != nil {
		return nil, e
	}
	return claims, tx.Commit(ctx)
}
func readClaims(ctx context.Context, tx pgx.Tx, ids []string) ([]catalog.Claim, error) {
	out := make([]catalog.Claim, 0, len(ids))
	for _, id := range ids {
		var c catalog.Claim
		e := tx.QueryRow(ctx, `SELECT claim_id,owner_id,scope_type,scope_id,fact_key,source_id,source_revision,value_json,summary,status,extraction_confidence,supersedes_id,accepted_by,event_time FROM memory_claims WHERE claim_id=$1`, id).Scan(&c.ID, &c.OwnerID, &c.ScopeType, &c.ScopeID, &c.Key, &c.SourceID, &c.SourceRevision, &c.Value, &c.Summary, &c.Status, &c.Confidence, &c.SupersedesID, &c.AcceptedBy, &c.EventTime)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, nil
}
