package artifacts

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// ReportID is derived from the immutable publication identity, not from an
// adapter part. All channels refer to one canonical report artifact.
func ReportID(publication string) string {
	return "communication-report:" + strings.TrimPrefix(publication, "publication:")
}

func SaveReportTx(ctx context.Context, tx pgx.Tx, publication, runID, key string, args communicationcontrol.Args) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || tx == nil || token.RunID != runID || token.SessionKey != key || !strings.HasPrefix(publication, "publication:") {
		return ErrMediaDenied
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return ErrMediaDenied
	}
	if _, err = communicationcontrol.DecodeArgs("communication.report", string(raw)); err != nil {
		return ErrMediaDenied
	}
	_, err = tx.Exec(ctx, `INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,title,summary,content_json,content_format,source_type,source_ref) VALUES($1,$2,$3,'communication_report',$4,$5,$6::jsonb,'json','publication',$7)`, ReportID(publication), runID, key, args.Title, args.Summary, raw, publication)
	return err
}
