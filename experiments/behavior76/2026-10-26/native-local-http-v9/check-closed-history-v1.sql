\set ON_ERROR_STOP on
BEGIN;
SET LOCAL statement_timeout='15s';
CREATE TEMP TABLE checker_v9_current(raw text) ON COMMIT DROP;
CREATE TEMP TABLE checker_v9_prior(raw text) ON COMMIT DROP;
\copy checker_v9_current(raw) FROM '/src20/.cache/behavior76/native-local-http-v9/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_v9_prior(raw) FROM '/src20/.cache/behavior76/native-local-model-v8/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_v9_current)
 SELECT count(*)=723 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current723 rows differ'; END IF;
 SELECT count(*)=707 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.raw::jsonb) INTO old_rows
 FROM checker_v9_prior e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.raw::jsonb->>'window_id' AND a.n=(e.raw::jsonb->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original707 rows changed'; END IF;
 IF NOT (SELECT count(*)=10 AND bool_and(closed) AND sum(admitted)=723 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=723 AND count(total_tokens)=713 AND sum(total_tokens)=5626388 AND count(*)FILTER(WHERE total_tokens IS NULL)=10 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=16 AND min(n)=1 AND max(n)=16 AND count(total_tokens)=15 AND sum(total_tokens)=115562 AND count(*)FILTER(WHERE total_tokens IS NULL)=1 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T092444') THEN RAISE EXCEPTION 'V9 accounting mismatch'; END IF;
 IF NOT (SELECT closed AND admitted=16 FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T092444') THEN RAISE EXCEPTION 'V9 not closed'; END IF;
 IF NOT (SELECT kind='start' AND status='transport_error' AND stage='response_stream' AND milliseconds=61986 AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL AND jsonb_array_length(events)=4 AND events->0->>'type'='run_started' AND events->1->>'type'='assistant_delta' AND events->2->>'type'='assistant_delta' AND events->3='{"type":"transport_error","error_code":"provider_protocol_error","safe_reason":"stream_finish_rejected","http_status":0}'::jsonb FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T092444' AND n=16) THEN RAISE EXCEPTION 'Last exact protocol metadata differs'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_LOCAL_HTTP_V9:723 rows equal export; original707 unchanged;10closed windows;713measured/5626388known/10NULL; v9:16admitted/15measured/115562known/1NULL; noentry17; n16 retains rejected finish after two deltas, usage/modelversionNULL, exact rejected finish reason UNKNOWN';
END $proof$;
ROLLBACK;
