\set ON_ERROR_STOP on
BEGIN;
SET LOCAL statement_timeout='15s';
CREATE TEMP TABLE checker_v8_current(raw text) ON COMMIT DROP;
CREATE TEMP TABLE checker_v8_prior(raw text) ON COMMIT DROP;
\copy checker_v8_current(raw) FROM '/src20/.cache/behavior76/native-local-model-v8/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_v8_prior(raw) FROM '/src20/.cache/behavior76/native-day-v7-r2/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_v8_current)
 SELECT count(*)=707 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current707 rows differ'; END IF;
 SELECT count(*)=643 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.raw::jsonb) INTO old_rows
 FROM checker_v8_prior e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.raw::jsonb->>'window_id' AND a.n=(e.raw::jsonb->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original643 rows changed'; END IF;
 IF NOT (SELECT count(*)=9 AND bool_and(closed) AND sum(admitted)=707 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=707 AND count(total_tokens)=698 AND sum(total_tokens)=5510826 AND count(*)FILTER(WHERE total_tokens IS NULL)=9 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=64 AND min(n)=1 AND max(n)=64 AND count(total_tokens)=63 AND sum(total_tokens)=600214 AND count(*)FILTER(WHERE total_tokens IS NULL)=1 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T065844') THEN RAISE EXCEPTION 'V8 accounting mismatch'; END IF;
 IF NOT (SELECT closed AND admitted=64 FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T065844') THEN RAISE EXCEPTION 'V8 not closed'; END IF;
 IF NOT (SELECT kind='submit' AND status='transport_error' AND stage='http_round_trip' AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL AND events='[{"type":"run_started"},{"type":"transport_error","error_code":"transport_connection_error","safe_reason":"","http_status":0}]'::jsonb FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T065844' AND n=64) THEN RAISE EXCEPTION 'Last exact transport metadata differs'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_LOCAL_MODEL_V8:707 current rows equal export; original643 unchanged;9closed windows;698measured/5510826known/9NULL; v8:64admitted/63measured/600214known/1NULL; noentry65; n64 retains HTTP phase connection failure, no usable response/usage/version';
END $proof$;
ROLLBACK;
