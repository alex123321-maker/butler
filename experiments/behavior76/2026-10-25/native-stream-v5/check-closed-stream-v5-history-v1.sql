\set ON_ERROR_STOP on
BEGIN;
SET LOCAL statement_timeout='15s';
CREATE TEMP TABLE checker_stream_current(raw text) ON COMMIT DROP;
CREATE TEMP TABLE checker_stream_prior(raw text) ON COMMIT DROP;
\copy checker_stream_current(raw) FROM '/src20/.cache/behavior76/native-stream-v5/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_stream_prior(raw) FROM '/src20/.cache/behavior76/native-local-v4-r2/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_stream_current)
 SELECT count(*)=506 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current506 rows differ'; END IF;
 SELECT count(*)=405 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.raw::jsonb) INTO old_rows
 FROM checker_stream_prior e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.raw::jsonb->>'window_id' AND a.n=(e.raw::jsonb->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original405 rows changed'; END IF;
 IF NOT (SELECT count(*)=6 AND bool_and(closed) AND sum(admitted)=506 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=506 AND count(total_tokens)=500 AND sum(total_tokens)=3832577 AND count(*)FILTER(WHERE total_tokens IS NULL)=6 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=101 AND min(n)=1 AND max(n)=101 AND count(total_tokens)=100 AND sum(total_tokens)=748105 AND count(*)FILTER(WHERE total_tokens IS NULL)=1 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-stream-20261025T231554') THEN RAISE EXCEPTION 'Stream accounting mismatch'; END IF;
 IF NOT (SELECT kind='submit' AND status='cancelled' AND stage='http_round_trip' AND milliseconds=19851 AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL AND jsonb_array_length(events)=2 AND events->0->>'type'='run_started' AND events->1->>'type'='transport_error' AND events->1->>'error_code'='provider_timeout' FROM public.behavior76_attempts WHERE window_id='issue76-fixture-stream-20261025T231554' AND n=101) THEN RAISE EXCEPTION 'Last admission metadata changed'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_STREAM:506 rows equal sealed current export; original405 unchanged;6closed windows;500measured/3832577known/6NULL; stream101/100/748105/1NULL; no entry102';
END $proof$;
ROLLBACK;
