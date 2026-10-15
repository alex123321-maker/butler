package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

type PolicyRequest struct {
	SessionKey string                  `json:"session_key"`
	Policy     jobs.ModelPolicyRequest `json:"policy"`
}
type Policies struct {
	Catalog *catalog.Store
	Jobs    *jobs.Store
}

// Create commits the primary snapshot and its complete dynamic privacy manifest
// atomically. Policy admission never exposes a root lacking its source fence.
func (p *Policies) Create(ctx context.Context, q PolicyRequest) (jobs.ModelPolicy, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || p == nil || p.Catalog == nil || p.Jobs == nil || q.SessionKey == "" || q.Policy.Job.OriginalRunID == "" || len(q.Policy.Job.Sources) != 0 || len(q.Policy.ID) < 8 || len(q.Policy.ID) > 128 {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	if err := p.Catalog.CheckRestore(ctx); err != nil {
		return jobs.ModelPolicy{}, err
	}
	tx, err := p.Catalog.Pool().Begin(ctx)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	defer tx.Rollback(ctx)
	var owner, task, session string
	err = tx.QueryRow(ctx, `SELECT s.user_id,r.session_key,COALESCE(r.task_id,'') FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, q.Policy.Job.OriginalRunID).Scan(&owner, &session, &task)
	if err != nil || owner != a.OwnerID || session != q.SessionKey || task != q.Policy.Job.TaskID {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	if _, err = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, session); err != nil {
		return jobs.ModelPolicy{}, err
	}
	d, _, err := readDocumentTx(ctx, tx, session, 0)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if len(d.Events) == 0 {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	body, _ := json.Marshal(d)
	if len(body) > maxSourceBytes {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	idHash := sha256.Sum256([]byte(a.OwnerID + ":" + q.Policy.ID))
	id := "source_" + hex.EncodeToString(idHash[:])
	origin := content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{id}, Sensitivity: content.Private}
	originJSON, _ := json.Marshal(origin)
	locator, _ := json.Marshal(map[string]any{"summary_version": 1, "session_key": session})
	sum := sha256.Sum256(body)
	tag, err := tx.Exec(ctx, `INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key)VALUES($1,$2,'document','session',$3,$4)ON CONFLICT DO NOTHING`, id, a.OwnerID, session, "summary-policy:"+q.Policy.ID)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if tag.RowsAffected() != 1 {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author)VALUES($1,1,$2,'Primary transcript for verified compaction',$3,$4,$5,'trusted-transcript-adapter')ON CONFLICT DO NOTHING`, id, hex.EncodeToString(sum[:]), string(body), locator, originJSON); err != nil {
		return jobs.ModelPolicy{}, err
	}
	if err = pinOriginalsTx(ctx, tx, id, session, owner, task, d.SourceRevision); err != nil {
		return jobs.ModelPolicy{}, err
	}
	sources, err := p.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{{ID: id}}, "memory_job_create")
	if err != nil || len(sources) != 1 {
		return jobs.ModelPolicy{}, jobs.ErrClaim
	}
	if err = tx.Commit(ctx); err != nil {
		return jobs.ModelPolicy{}, err
	}
	src := sources[0]
	q.Policy.Handler = HandlerName
	q.Policy.Trigger = "source_changed"
	q.Policy.Job.Sources = []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}
	return p.Jobs.CreateModelPolicy(ctx, q.Policy)
}
