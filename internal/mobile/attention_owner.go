package mobile

import "context"

type AttentionRule struct {
	ID      string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
	Body    string `json:"body,omitempty"`
}
type AttentionView struct {
	Enabled    bool            `json:"enabled"`
	Pending    int             `json:"pending"`
	Remembered int             `json:"remembered"`
	Noise      int             `json:"noise"`
	Rules      []AttentionRule `json:"rules"`
}

func (s *Store) Attention(ctx context.Context, reveal bool) (AttentionView, error) {
	out := AttentionView{Rules: []AttentionRule{}}
	own, err := owner(ctx)
	if err != nil {
		return out, err
	}
	if err = s.Catalog.CheckRestore(ctx); err != nil {
		return out, err
	}
	err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mobile_agent_access WHERE owner_id=$1 AND enabled),count(*) FILTER(WHERE state='pending'),count(*) FILTER(WHERE state='remembered'),count(*) FILTER(WHERE state='noise') FROM mobile_messages WHERE owner_id=$1`, own).Scan(&out.Enabled, &out.Pending, &out.Remembered, &out.Noise)
	if err != nil {
		return out, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT n.rule_id,n.enabled,CASE WHEN $2 THEN r.body ELSE '' END FROM mobile_noise_rules n JOIN memory_sources s ON s.source_id=n.rule_id AND s.status='active' JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE n.owner_id=$1 ORDER BY n.created_at DESC LIMIT 100`, own, reveal)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v AttentionRule
		if err = rows.Scan(&v.ID, &v.Enabled, &v.Body); err != nil {
			return out, err
		}
		out.Rules = append(out.Rules, v)
	}
	return out, rows.Err()
}
func (s *Store) ChangeNoiseRule(ctx context.Context, id string, enabled bool) error {
	own, err := owner(ctx)
	if err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE mobile_noise_rules SET enabled=$3 WHERE rule_id=$1 AND owner_id=$2`, id, own, enabled)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return err
}
