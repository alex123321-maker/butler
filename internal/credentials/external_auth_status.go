package credentials

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"net/url"

	"github.com/jackc/pgx/v5"
)

func (v *Vault) ExternalRPHost(host string) bool {
	if v == nil || v.external == nil {
		return false
	}
	u, e := url.Parse(v.external.profile.RPURL)
	return e == nil && host == u.Host
}

func ExternalPending(state string) bool {
	return state == "pending" || state == "approved" || state == "launched" || state == "exchanging"
}

func (v *Vault) ExternalProfileConfigured(ctx context.Context) bool {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return false
	}
	defer tx.Rollback(ctx)
	p, e := v.externalProfileTx(ctx, tx)
	return e == nil && p.Validate() == nil && tx.Commit(ctx) == nil
}

func (v *Vault) ExternalRequests(ctx context.Context, owner string) ([]AccessRequest, error) {
	out := []AccessRequest{}
	// Older narrow fixture schemas have no external capability. Production
	// always applies ordered migrations before enabling either composition.
	var exists bool
	if v.Pool.QueryRow(ctx, `SELECT to_regclass('browser_external_auth_requests') IS NOT NULL`).Scan(&exists) != nil {
		return nil, ErrVault
	}
	if !exists {
		return out, nil
	}
	rows, e := v.Pool.Query(ctx, `SELECT q.id,q.live_state,c.title||' · '||c.alias FROM browser_external_auth_state q JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id JOIN accepted_inputs i ON i.input_id=q.input_id AND i.run_id=q.run_id AND i.checkpoint='awaiting_interaction' AND i.checkpoint_json->'interaction'->>'interaction_id'=q.id WHERE q.owner_id=$1 AND q.live_state IN ('pending','approved','launched','exchanging') ORDER BY q.created_at LIMIT 50`, owner)
	if e != nil {
		return nil, ErrVault
	}
	type item struct{ id, state, label string }
	var items []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.id, &x.state, &x.label) != nil {
			rows.Close()
			return nil, ErrVault
		}
		items = append(items, x)
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return nil, ErrVault
	}
	defer tx.Rollback(ctx)
	for _, x := range items {
		q, e := v.loadExternalTx(ctx, tx, x.id)
		if e != nil || q.p.OwnerID != owner {
			return nil, ErrVault
		}
		r := q.metadata()
		r.State = x.state
		r.RecordLabel = x.label
		out = append(out, r)
	}
	if tx.Commit(ctx) != nil {
		return nil, ErrVault
	}
	return out, nil
}

// ExternalStateTx returns a retained fact only for its exact caller identity.
// It never decrypts challenge/cookie material or returns a credential alias.
func (v *Vault) ExternalStateTx(ctx context.Context, tx pgx.Tx, p Principal, id string) (AccessRequest, string, error) {
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || q.p != p || currentTx(ctx, tx, p) != nil {
		return AccessRequest{}, "", ErrVault
	}
	var state, reference string
	if tx.QueryRow(ctx, `SELECT q.live_state,COALESCE(r.rp_reference,'') FROM browser_external_auth_state q LEFT JOIN browser_external_auth_results r ON r.request_id=q.id WHERE q.id=$1`, id).Scan(&state, &reference) != nil {
		return AccessRequest{}, "", ErrVault
	}
	r := q.metadata()
	r.State = state
	if state != "verified" {
		reference = ""
	}
	return r, reference, nil
}

func (v *Vault) externalStatus(ctx context.Context, p Principal, id string) (AccessRequest, error) {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return AccessRequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	r, _, e := v.ExternalStateTx(ctx, tx, p, id)
	if e != nil || tx.Commit(ctx) != nil {
		return AccessRequest{}, ErrVault
	}
	return r, nil
}

// SweepExternal retains a lost exchange as UNKNOWN only after the bounded
// provider/probe execution window has settled. It never replays an exchange.
func (v *Vault) SweepExternal(ctx context.Context) error {
	var exists bool
	if v.Pool.QueryRow(ctx, `SELECT to_regclass('browser_external_auth_requests') IS NOT NULL`).Scan(&exists) != nil {
		return ErrVault
	}
	if !exists {
		return nil
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	lost, e := tx.Query(ctx, `INSERT INTO browser_external_auth_results(request_id,outcome) SELECT e.request_id,'unknown' FROM browser_external_auth_exchanges e WHERE e.claimed_at<clock_timestamp()-interval '75 seconds' AND NOT EXISTS(SELECT 1 FROM browser_external_auth_results r WHERE r.request_id=e.request_id) ON CONFLICT DO NOTHING RETURNING request_id`)
	if e != nil {
		return ErrVault
	}
	var lostIDs []string
	for lost.Next() {
		var id string
		if lost.Scan(&id) != nil {
			lost.Close()
			return ErrVault
		}
		lostIDs = append(lostIDs, id)
	}
	lost.Close()
	if lost.Err() != nil {
		return ErrVault
	}
	for _, id := range lostIDs {
		if actionauth.FinishExternalAuthTx(ctx, tx, id) != nil {
			return ErrVault
		}
	}
	rows, e := tx.Query(ctx, `SELECT q.run_id,q.id FROM browser_external_auth_state q JOIN accepted_inputs i ON i.input_id=q.input_id AND i.run_id=q.run_id WHERE q.live_state NOT IN ('pending','approved','launched','exchanging') AND i.checkpoint='awaiting_interaction' AND i.checkpoint_json->'interaction'->>'interaction_id'=q.id AND i.status IN ('running','waiting_for_user','blocked') FOR UPDATE OF i SKIP LOCKED`)
	if e != nil {
		return ErrVault
	}
	type wait struct{ run, id string }
	var waits []wait
	for rows.Next() {
		var w wait
		if rows.Scan(&w.run, &w.id) != nil {
			rows.Close()
			return ErrVault
		}
		waits = append(waits, w)
	}
	rows.Close()
	if rows.Err() != nil {
		return ErrVault
	}
	for _, w := range waits {
		if v.AfterResponse != nil && v.AfterResponse(ctx, tx, w.run, w.id) != nil {
			return ErrVault
		}
	}
	if tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}
