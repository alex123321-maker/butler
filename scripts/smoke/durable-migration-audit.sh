#!/bin/sh
set -eu
src=${1:?repository directory required}
container=${2:?PostgreSQL container name required}
db=${3:?fresh test database name required}
case "$db" in
  butler_durable_migrations_*) ;;
  *) printf '%s\n' 'test database must start with butler_durable_migrations_' >&2; exit 1 ;;
esac
# Refuse an existing database; never drop or reset another suite's state.
docker exec "$container" createdb -U butler "$db"
apply() { docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" < "$1"; }
for migration in "$src"/migrations/*.up.sql; do
  number=$(basename "$migration" | cut -d_ -f1)
  if [ "$number" -ge 32 ]; then continue; fi
  apply "$migration"
done
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
INSERT INTO sessions(session_key,user_id,channel) VALUES('migration-sentinel','owner','web');
INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)
VALUES('migration-run','migration-sentinel','legacy-event','waiting','mode1','awaiting_approval','fixture',now(),now());
INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('migration-message','migration-sentinel','migration-run','user','Preserve historical transcript');
INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('migration-task','migration-sentinel','owner','Legacy waiting work','waiting_for_user');
UPDATE runs SET task_id='migration-task',task_revision=1 WHERE run_id='migration-run';
INSERT INTO approvals(approval_id,run_id,session_key,tool_call_id,status,requested_via,tool_name)
VALUES('migration-approval','migration-run','migration-sentinel','legacy-tool','pending','web','fixture.external');
SQL
apply "$src/migrations/032_durable_ingress.up.sql"
apply "$src/migrations/033_durable_approval_continuations.up.sql"
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
DO $$ BEGIN
 IF (SELECT status FROM approvals WHERE approval_id='migration-approval') <> 'expired' THEN RAISE EXCEPTION 'legacy pending permission survived'; END IF;
 IF (SELECT status FROM tasks WHERE task_id='migration-task') <> 'blocked' THEN RAISE EXCEPTION 'legacy task not blocked'; END IF;
 IF EXISTS(SELECT 1 FROM approval_actions) THEN RAISE EXCEPTION 'migration invented action authority'; END IF;
END $$;
SQL
apply "$src/migrations/033_durable_approval_continuations.down.sql"
apply "$src/migrations/032_durable_ingress.down.sql"
apply "$src/migrations/032_durable_ingress.up.sql"
apply "$src/migrations/033_durable_approval_continuations.up.sql"
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
DO $$ BEGIN
 IF (SELECT count(*) FROM runs WHERE run_id='migration-run') <> 1 THEN RAISE EXCEPTION 'lost historical run'; END IF;
 IF (SELECT count(*) FROM messages WHERE message_id='migration-message' AND content='Preserve historical transcript') <> 1 THEN RAISE EXCEPTION 'lost transcript'; END IF;
 IF (SELECT status FROM approvals WHERE approval_id='migration-approval') <> 'expired' THEN RAISE EXCEPTION 'rollback revived permission'; END IF;
END $$;
SELECT 'durable migration up/down/up and legacy history preservation passed' AS result;
SQL
