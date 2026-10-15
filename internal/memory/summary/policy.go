package summary

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
)

type PolicyRequest struct {
	SessionKey string                  `json:"session_key"`
	Policy     jobs.ModelPolicyRequest `json:"policy"`
}
type Policies struct {
	Catalog *catalog.Store
	Jobs    *jobs.Store
}

// Create snapshots only an authenticated owner's actual primary transcript.
// The caller supplies finite allowance/deadline/privacy; no default mandate or
// background authority is inferred from a foreground run or its completion.
func (p *Policies) Create(ctx context.Context, q PolicyRequest) (jobs.ModelPolicy, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || p == nil || p.Catalog == nil || p.Jobs == nil || q.SessionKey == "" || q.Policy.Job.OriginalRunID == "" || len(q.Policy.Job.Sources) != 0 {
		return jobs.ModelPolicy{}, jobs.ErrClaim
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
	if err = tx.Commit(ctx); err != nil {
		return jobs.ModelPolicy{}, err
	}
	locator, _ := json.Marshal(map[string]any{"summary_version": 1, "session_key": session})
	src, err := p.Catalog.Put(ctx, catalog.PutSource{ExternalKey: "summary-policy:" + q.Policy.ID, Kind: "document", ScopeType: "session", ScopeID: session, Title: "Primary transcript for verified compaction", Body: string(body), Locator: locator})
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	if _, err = p.Catalog.Pool().Exec(ctx, `INSERT INTO memory_summary_sources(source_id,session_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, src.ID, session); err != nil {
		return jobs.ModelPolicy{}, err
	}
	q.Policy.Handler = HandlerName
	q.Policy.Trigger = "source_changed"
	q.Policy.Job.Sources = []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}
	return p.Jobs.CreateModelPolicy(ctx, q.Policy)
}
