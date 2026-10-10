package artifacts

import (
	"context"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// SaveFinalTx inserts a required immutable assistant artifact in the owner's transaction.
func (r *PostgresRepository) SaveFinalTx(ctx context.Context, tx pgx.Tx, p CreateParams) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != p.RunID || token.SessionKey != p.SessionKey {
		return ownership.ErrStale
	}
	_, err := tx.Exec(ctx, `INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,title,summary,content_text,content_json,content_format,source_type,source_ref,created_at,updated_at) VALUES($1,$2,$3,'assistant_final','Assistant final',$4,$4,'{}','text','message',$5,$6,$6)`, p.ArtifactID, p.RunID, p.SessionKey, p.ContentText, p.SourceRef, p.CreatedAt)
	return err
}
