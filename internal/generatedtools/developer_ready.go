package generatedtools

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckDeveloperReady checks the owned accepted-role and reference-only return
// schema before an operator enables the optional developer bridge. It grants no
// caller authority and does not replace the per-call current lifecycle guards.
func CheckDeveloperReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrAccess
	}
	var immutable bool
	err := pool.QueryRow(ctx, `WITH owned(name,oid) AS (
 SELECT name,to_regclass(format('%I.%I',current_schema(),name)) FROM (VALUES('converter_developer_snapshots'),('converter_developer_returns')) n(name)
), required(table_name,column_name,type_name) AS (
 SELECT 'converter_developer_snapshots',n,'pg_catalog.text' FROM unnest(ARRAY['task_id','owner_id','parent_task_id','execution_action_id','effect_id','authorization_id','action_hash','args_hash','contract_hash','snapshot_hash']) n
 UNION ALL SELECT 'converter_developer_snapshots',n,'pg_catalog.int8' FROM unnest(ARRAY['parent_revision','parent_constraints_revision']) n
 UNION ALL SELECT 'converter_developer_snapshots','accepted_json','pg_catalog.jsonb'
 UNION ALL SELECT 'converter_developer_snapshots','created_at','pg_catalog.timestamptz'
 UNION ALL SELECT 'converter_developer_returns',n,'pg_catalog.text' FROM unnest(ARRAY['child_task_id','owner_id','parent_task_id','result_id','child_run_id','digest','projection_hash']) n
 UNION ALL SELECT 'converter_developer_returns','projection_json','pg_catalog.jsonb'
 UNION ALL SELECT 'converter_developer_returns','created_at','pg_catalog.timestamptz'
)
SELECT NOT EXISTS(SELECT 1 FROM owned WHERE oid IS NULL)
 AND NOT EXISTS(SELECT 1 FROM required r JOIN owned o ON o.name=r.table_name LEFT JOIN pg_attribute a ON a.attrelid=o.oid AND a.attname=r.column_name AND NOT a.attisdropped WHERE a.attname IS NULL OR a.atttypid<>to_regtype(r.type_name) OR NOT a.attnotnull)
 AND (SELECT count(*) FROM pg_trigger t JOIN owned o ON o.oid=t.tgrelid WHERE NOT t.tgisinternal AND t.tgenabled='O' AND t.tgfoid=to_regprocedure(format('%I.%I()',current_schema(),'converter_immutable_record')) AND ((o.name='converter_developer_snapshots' AND t.tgname='converter_developer_immutable') OR (o.name='converter_developer_returns' AND t.tgname='converter_return_immutable')))=2`).Scan(&immutable)
	if err != nil || !immutable {
		return ErrAccess
	}
	return nil
}
