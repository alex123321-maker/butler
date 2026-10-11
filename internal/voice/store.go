// Package voice keeps recordings and transcriptions as private source data.
// Only an authenticated owner can review a command for the ordinary input path.
package voice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
	"time"
)

var ErrDenied = errors.New("voice data or current owner review unavailable")

const Version = "butler-voice-pcm-v1"
const Purpose = "voice_asr"

type Role string

const (
	OwnerCommand Role = "owner_command"
	Note         Role = "note"
	External     Role = "external"
)

type Clip struct {
	ID            string             `json:"clip_id"`
	OwnerID       string             `json:"owner_id"`
	Role          Role               `json:"role"`
	Source        catalog.SourceRef  `json:"source"`
	OriginalHash  string             `json:"original_hash"`
	State         string             `json:"state"`
	Revision      int64              `json:"revision"`
	Transcript    *catalog.SourceRef `json:"transcript_source,omitempty"`
	Reviewed      *catalog.SourceRef `json:"reviewed_source,omitempty"`
	JobID         string             `json:"job_id"`
	InputID       string             `json:"input_id"`
	ErrorCode     string             `json:"error_code"`
	Expires       time.Time          `json:"expires_at"`
	Text          string             `json:"transcript"`
	ControlTarget *Target            `json:"control_target,omitempty"`
}
type Upload struct {
	WAV            []byte
	Role           Role
	RetentionHours uint32
	Secret         bool
}
type Store struct {
	Catalog *catalog.Store
	Jobs    *jobs.Store
}

func encode(v any) []byte { b, _ := json.Marshal(v); return b }
func ref(s catalog.Source) catalog.SourceRef {
	return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
}
func (s *Store) Upload(ctx context.Context, u Upload) (Clip, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.Catalog == nil || u.Secret || u.RetentionHours < 1 || u.RetentionHours > 168 || (u.Role != OwnerCommand && u.Role != Note && u.Role != External) {
		return Clip{}, ErrDenied
	}
	if _, e := audio.Decode(u.WAV, 16000, 120); e != nil {
		return Clip{}, e
	}
	key := sha256.Sum256([]byte(actor.OwnerID + "\x00" + actor.DeliveryID))
	id := "voice_" + hex.EncodeToString(key[:])
	sum := sha256.Sum256(u.WAV)
	original := hex.EncodeToString(sum[:])
	origin := content.Origin{Kind: content.Document, Sensitivity: content.Private}
	locator := encode(map[string]any{"format": "pcm16-wav", "original_hash": original, "role": u.Role, "parser_version": Version, "retention_hours": u.RetentionHours})
	src, e := s.Catalog.Put(ctx, catalog.PutSource{ExternalKey: id, Kind: "document", ScopeType: "user", ScopeID: actor.OwnerID, Title: "voice recording", Body: original, Locator: locator, Origin: origin, Author: "owner upload"})
	if e != nil {
		return Clip{}, e
	}
	r := ref(src)
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return Clip{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{r}, "memory_ingest"); e != nil {
		return Clip{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_source_assets(source_id,source_revision,format,path,original_hash,original_bytes,parser_version,method,status) VALUES($1,$2,'wav','voice.wav',$3,$4,$5,'owner_pcm','ready') ON CONFLICT(source_id,source_revision) DO NOTHING`, r.ID, r.Revision, original, u.WAV, Version); e != nil {
		return Clip{}, e
	}
	var exact bool
	if tx.QueryRow(ctx, `SELECT original_hash=$3 AND parser_version=$4 AND status='ready' FROM memory_source_assets WHERE source_id=$1 AND source_revision=$2`, r.ID, r.Revision, original, Version).Scan(&exact) != nil || !exact {
		return Clip{}, ErrDenied
	}
	if _, e = tx.Exec(ctx, `INSERT INTO voice_clips(clip_id,owner_id,role,source_ref,original_hash,state,expires_at) VALUES($1,$2,$3,$4,$5,'stored',clock_timestamp()+$6*INTERVAL '1 hour') ON CONFLICT(clip_id) DO NOTHING`, id, actor.OwnerID, u.Role, encode(r), original, u.RetentionHours); e != nil {
		return Clip{}, e
	}
	c, e := load(ctx, tx, id, false)
	if e != nil || c.OwnerID != actor.OwnerID || c.Role != u.Role || c.Source != r || c.OriginalHash != original {
		return Clip{}, ErrDenied
	}
	return c, tx.Commit(ctx)
}
func load(ctx context.Context, tx pgx.Tx, id string, lock bool) (Clip, error) {
	var c Clip
	var source, transcript, reviewed, controlTarget []byte
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT clip_id,owner_id,role,source_ref,original_hash,state,revision,transcript_ref,reviewed_ref,job_id,input_id,error_code,expires_at,control_target FROM voice_clips WHERE clip_id=$1`+clause, id).Scan(&c.ID, &c.OwnerID, &c.Role, &source, &c.OriginalHash, &c.State, &c.Revision, &transcript, &reviewed, &c.JobID, &c.InputID, &c.ErrorCode, &c.Expires, &controlTarget)
	if e == nil {
		e = json.Unmarshal(source, &c.Source)
		if e == nil && len(transcript) > 0 {
			e = json.Unmarshal(transcript, &c.Transcript)
		}
		if e == nil && len(reviewed) > 0 {
			e = json.Unmarshal(reviewed, &c.Reviewed)
		}
		if e == nil && len(controlTarget) > 0 {
			e = json.Unmarshal(controlTarget, &c.ControlTarget)
		}
	}
	return c, e
}
func (s *Store) read(ctx context.Context, id, purpose string) (Clip, []byte, error) {
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return Clip{}, nil, e
	}
	defer tx.Rollback(ctx)
	c, e := load(ctx, tx, id, false)
	if e != nil {
		return Clip{}, nil, ErrDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT clock_timestamp()<$1`, c.Expires).Scan(&current) != nil || !current || c.State == "deleted" || c.State == "cancelled" {
		return Clip{}, nil, ErrDenied
	}
	refs := []catalog.SourceRef{c.Source}
	if c.Transcript != nil {
		refs = append(refs, *c.Transcript)
	}
	if c.Reviewed != nil {
		refs = append(refs, *c.Reviewed)
	}
	srcs, e := s.Catalog.AccessTx(ctx, tx, refs, purpose)
	if e != nil {
		return Clip{}, nil, e
	}
	for _, src := range srcs {
		if c.Transcript != nil && src.ID == c.Transcript.ID {
			c.Text = src.Body
		}
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT original_bytes FROM memory_source_assets WHERE source_id=$1 AND source_revision=$2 AND status='ready' AND original_hash=$3`, c.Source.ID, c.Source.Revision, c.OriginalHash).Scan(&raw) != nil {
		return Clip{}, nil, ErrDenied
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != c.OriginalHash {
		return Clip{}, nil, ErrDenied
	}
	if _, e = audio.Decode(raw, 16000, 120); e != nil {
		return Clip{}, nil, ErrDenied
	}
	return c, raw, tx.Commit(ctx)
}
func (s *Store) Get(ctx context.Context, id string) (Clip, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return Clip{}, ErrDenied
	}
	c, _, e := s.read(ctx, id, "memory_read")
	if e == nil && c.OwnerID != a.OwnerID {
		return Clip{}, ErrDenied
	}
	return c, e
}
func (s *Store) Original(ctx context.Context, id string) ([]byte, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, ErrDenied
	}
	c, w, e := s.read(ctx, id, "memory_read")
	if e == nil && c.OwnerID != a.OwnerID {
		return nil, ErrDenied
	}
	return w, e
}

// Enqueue stores an explicit finite local voice allowance under owner proof.
// No job is granted by changing an enabled flag or by a prior completed run.
func (s *Store) Enqueue(ctx context.Context, id string, allowance budget.Units) (jobs.Job, error) {
	c, e := s.Get(ctx, id)
	if e != nil || s.Jobs == nil || (c.State != "stored" && c.State != "failed") {
		return jobs.Job{}, ErrDenied
	}
	jobID, e := budget.NewOperationID()
	if e != nil {
		return jobs.Job{}, e
	}
	deadline := time.Now().Add(10 * time.Minute)
	if c.Expires.Before(deadline) {
		deadline = c.Expires
	}
	return s.Jobs.EnqueueWithPublication(ctx, jobs.Request{ID: jobID, Kind: "ingest", Purpose: Purpose, Sources: []catalog.SourceRef{c.Source}, Allowance: allowance, Deadline: deadline}, func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
		tag, e := tx.Exec(ctx, `UPDATE voice_clips SET state='pending',job_id=$2,revision=revision+1,error_code='',updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$3 AND state IN ('stored','failed') AND expires_at>clock_timestamp()`, id, j.ID, c.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrDenied
		}
		return nil
	})
}

// Cancel fences this clip's late ASR callback; it does not implement task Stop.
func (s *Store) Cancel(ctx context.Context, id string, revision int64) error {
	c, e := s.Get(ctx, id)
	if e != nil {
		return e
	}
	tag, e := s.Catalog.Pool().Exec(ctx, `UPDATE voice_clips SET state='cancelled',revision=revision+1,updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$2 AND owner_id=$3 AND state NOT IN ('submitted','deleted')`, id, revision, c.OwnerID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return nil
}
func (s *Store) Delete(ctx context.Context, id string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return ErrDenied
	}
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return e
	}
	c, e := load(ctx, tx, id, false)
	_ = tx.Rollback(ctx)
	if e != nil || c.OwnerID != a.OwnerID {
		return ErrDenied
	}
	if _, e = s.Catalog.Delete(ctx, c.Source.ID, "delete", c.Source.Revision); e != nil {
		return e
	}
	_, e = s.Catalog.Pool().Exec(ctx, `UPDATE voice_clips SET state='deleted',revision=revision+1,transcript_ref=NULL,reviewed_ref=NULL,updated_at=clock_timestamp() WHERE clip_id=$1`, id)
	return e
}
