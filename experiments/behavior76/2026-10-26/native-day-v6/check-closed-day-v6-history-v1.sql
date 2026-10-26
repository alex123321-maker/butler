\set ON_ERROR_STOP on
BEGIN;
SET LOCAL statement_timeout='15s';
CREATE TEMP TABLE checker_day_current(raw text) ON COMMIT DROP;
CREATE TEMP TABLE checker_day_prior(raw text) ON COMMIT DROP;
\copy checker_day_current(raw) FROM '/src20/.cache/behavior76/native-day-v6/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_day_prior(raw) FROM '/src20/.cache/behavior76/native-stream-v5/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_day_current)
 SELECT count(*)=640 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current640 rows differ'; END IF;
 SELECT count(*)=506 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.raw::jsonb) INTO old_rows
 FROM checker_day_prior e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.raw::jsonb->>'window_id' AND a.n=(e.raw::jsonb->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original506 rows changed'; END IF;
 IF NOT (SELECT count(*)=7 AND bool_and(closed) AND sum(admitted)=640 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=640 AND count(total_tokens)=633 AND sum(total_tokens)=4900143 AND count(*)FILTER(WHERE total_tokens IS NULL)=7 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=134 AND min(n)=1 AND max(n)=134 AND count(total_tokens)=133 AND sum(total_tokens)=1067566 AND count(*)FILTER(WHERE total_tokens IS NULL)=1 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T012812') THEN RAISE EXCEPTION 'Day accounting mismatch'; END IF;
 IF NOT (SELECT kind='start' AND status='transport_error' AND stage='response_stream' AND milliseconds=66018 AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL AND jsonb_array_length(events)=4 AND events->0->>'type'='run_started' AND events->1->>'type'='assistant_delta' AND events->2->>'type'='assistant_delta' AND events->3->>'type'='transport_error' AND events->3->>'error_code'='provider_protocol_error' FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T012812' AND n=134) THEN RAISE EXCEPTION 'Last admission metadata changed'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_DAY:640 rows equal sealed current export; original506 unchanged;7closed windows;633measured/4900143known/7NULL; day134/133/1067566/1NULL; no entry135; failed134 retains2text deltas/no tool/final';
END $proof$;
ROLLBACK;
