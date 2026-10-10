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

# Empty new stores can be rolled back; populated authoritative stores cannot.
for number in 034_atomic_results_outbox 035_external_effect_journal 036_task_owner_controls; do
  apply "$src/migrations/$number.up.sql"
done
for number in 036_task_owner_controls 035_external_effect_journal 034_atomic_results_outbox; do
  apply "$src/migrations/$number.down.sql"
done
apply "$src/migrations/034_atomic_results_outbox.up.sql"
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at)
VALUES('migration-action','migration-run','legacy-dispatched-tool','legacy-fingerprint','{}',true,now()+interval '1 hour');
INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state,dispatched_at,finished_at)
VALUES('migration-grant','migration-action','legacy-fingerprint','{}',now()+interval '1 hour','dispatch_completed',now(),now());
SQL
apply "$src/migrations/035_external_effect_journal.up.sql"
apply "$src/migrations/036_task_owner_controls.up.sql"
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
DO $$ BEGIN
 IF (SELECT state FROM action_effects WHERE action_id='migration-action') <> 'effect_unknown' THEN RAISE EXCEPTION 'legacy dispatch falsely confirmed'; END IF;
 IF (SELECT outcome FROM action_effect_dispatches WHERE authorization_id='migration-grant') <> 'dispatch_completed' THEN RAISE EXCEPTION 'lost spent dispatch'; END IF;
 IF (SELECT execution_policy FROM tasks WHERE task_id='migration-task') <> 'normal' THEN RAISE EXCEPTION 'legacy policy changed'; END IF;
 IF (SELECT count(*) FROM messages WHERE message_id='migration-message' AND content='Preserve historical transcript') <> 1 THEN RAISE EXCEPTION 'lost historical transcript'; END IF;
END $$;
INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('migration-final-message','migration-sentinel','migration-run','assistant','Saved fixture answer');
INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,content_text) VALUES('migration-final-artifact','migration-run','migration-sentinel','assistant_final','Saved fixture answer');
INSERT INTO run_final_results(result_id,run_id,session_key,task_id,task_revision,constraints_revision,artifact_id,message_id,content,content_hash,answer_state)
VALUES('migration-result','migration-run','migration-sentinel','migration-task',1,1,'migration-final-artifact','migration-final-message','Saved fixture answer','fixture-hash','final');
UPDATE tasks SET execution_policy='prepare_only' WHERE task_id='migration-task';
SQL
expect_refused() {
  if apply "$1" >/dev/null 2>&1; then
    printf '%s\n' 'populated authoritative migration incorrectly allowed downgrade' >&2
    exit 1
  fi
}
expect_refused "$src/migrations/036_task_owner_controls.down.sql"
expect_refused "$src/migrations/035_external_effect_journal.down.sql"
expect_refused "$src/migrations/034_atomic_results_outbox.down.sql"
docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U butler -d "$db" <<'SQL'
DO $$ BEGIN
 IF (SELECT count(*) FROM run_final_results WHERE result_id='migration-result') <> 1 THEN RAISE EXCEPTION 'rollback discarded result'; END IF;
 IF (SELECT state FROM action_effects WHERE action_id='migration-action') <> 'effect_unknown' THEN RAISE EXCEPTION 'rollback discarded effect history'; END IF;
 IF (SELECT execution_policy FROM tasks WHERE task_id='migration-task') <> 'prepare_only' THEN RAISE EXCEPTION 'rollback discarded owner restriction'; END IF;
END $$;
SELECT 'result/effect/control migrations: empty rollback, legacy unknown effects and populated downgrade refusal passed' AS result;
SQL
