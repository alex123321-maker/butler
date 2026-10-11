package singletabcontrol

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckSchema is startup validation of the owning immutable migrations063/064
// boundary. It does not assert metadata/runtime readiness or grant execution.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrDenied
	}
	var triggers int
	var body string
	err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(p.prosrc),'')
 FROM (VALUES ('guarded_single_tab_requests','guarded_request_immutable'),
 ('guarded_single_tab_selections','guarded_selection_immutable'),
 ('guarded_single_tab_choices','guarded_choice_immutable')) expected(relation,trigger_name)
 JOIN pg_trigger t ON t.tgrelid=to_regclass(expected.relation) AND t.tgname=expected.trigger_name
 JOIN pg_proc p ON p.oid=t.tgfoid
 WHERE NOT t.tgisinternal AND t.tgenabled IN ('O','A') AND p.proname='guarded_single_tab_immutable'
 AND t.tgtype=27 AND EXISTS(SELECT 1 FROM pg_attribute
 WHERE attrelid=to_regclass('execution_resource_dispatches') AND attname='single_tab_binding' AND NOT attisdropped)`).Scan(&triggers, &body)
	if err != nil || triggers != 3 || strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(immutableBody), " ") {
		return ErrDenied
	}
	return nil
}

// Pin the actual function, not only its name: a no-op replacement cannot make
// an owner endpoint ready. The integration fixture applies the real migration.
const immutableBody = `
BEGIN
 IF TG_TABLE_NAME='guarded_single_tab_requests' AND TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state')
   OR NOT ((OLD.state='pending' AND NEW.state IN ('selected','revoked')) OR (OLD.state='selected' AND NEW.state='revoked') OR OLD.state=NEW.state) THEN
   RAISE EXCEPTION 'guarded selection authority is immutable';
  END IF;
  RETURN NEW;
 END IF;
 RAISE EXCEPTION 'guarded selection history is immutable';
END `
