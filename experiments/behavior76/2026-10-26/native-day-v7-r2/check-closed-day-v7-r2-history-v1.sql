\set ON_ERROR_STOP on
BEGIN;
SET LOCAL statement_timeout='15s';
CREATE TEMP TABLE checker_day7_current(raw text) ON COMMIT DROP;
CREATE TEMP TABLE checker_day7_prior(raw text) ON COMMIT DROP;
\copy checker_day7_current(raw) FROM '/src20/.cache/behavior76/native-day-v7-r2/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_day7_prior(raw) FROM '/src20/.cache/behavior76/native-day-v6/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_day7_current)
 SELECT count(*)=643 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current643 rows differ'; END IF;
 SELECT count(*)=640 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.raw::jsonb) INTO old_rows
 FROM checker_day7_prior e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.raw::jsonb->>'window_id' AND a.n=(e.raw::jsonb->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original640 rows changed'; END IF;
 IF NOT (SELECT count(*)=8 AND bool_and(closed) AND sum(admitted)=643 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=643 AND count(total_tokens)=635 AND sum(total_tokens)=4910612 AND count(*)FILTER(WHERE total_tokens IS NULL)=8 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=3 AND min(n)=1 AND max(n)=3 AND count(total_tokens)=2 AND sum(total_tokens)=10469 AND count(*)FILTER(WHERE total_tokens IS NULL)=1 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T050548') THEN RAISE EXCEPTION 'Day7 accounting mismatch'; END IF;
 IF NOT (SELECT closed AND admitted=3 FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T050548') THEN RAISE EXCEPTION 'Day7 not closed'; END IF;
 IF NOT (SELECT kind='submit' AND status='transport_error' AND stage='response_stream' AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL AND EXISTS(SELECT 1 FROM jsonb_array_elements(events) e WHERE e->>'type'='transport_error' AND e->>'error_code'='provider_protocol_error' AND e->>'safe_reason'='stream_empty_answer') AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(events) e WHERE e->>'type' IN('assistant_final','tool_call_requested')) FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T050548' AND n=3) THEN RAISE EXCEPTION 'Last admission metadata differs'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_DAY7:643 rows equal sealed current export; original640 unchanged;8closed windows;635measured/4910612known/8NULL; day3/2/10469/1NULL; noentry4; failed3 retains local stream_empty_answer with no final/tool proposal';
END $proof$;
ROLLBACK;
