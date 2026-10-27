\set ON_ERROR_STOP on
CREATE TEMP TABLE checker_r3_final(raw text);
CREATE TEMP TABLE checker_r3_base(raw text);
\copy checker_r3_final(raw) FROM '/src20/.cache/behavior76/native-local-response-v10-r3/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_r3_base(raw) FROM '/src20/.cache/behavior76/native-local-response-v10-r2/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
BEGIN READ ONLY;
SET LOCAL statement_timeout='15s';
DO $proof$
DECLARE exact_rows boolean; old_rows boolean;
BEGIN
 WITH e AS (SELECT raw::jsonb value FROM checker_r3_final)
 SELECT count(*)=971 AND bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT exact_rows THEN RAISE EXCEPTION 'Current971 rows differ'; END IF;
 WITH e AS (SELECT raw::jsonb value FROM checker_r3_base)
 SELECT count(*)=774 AND bool_and(a.window_id IS NOT NULL AND to_jsonb(a)=e.value) INTO old_rows
 FROM e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT old_rows THEN RAISE EXCEPTION 'Original774 rows changed'; END IF;
 IF NOT (SELECT count(*)=12 AND bool_and(closed) AND sum(admitted)=971 FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Window closure mismatch'; END IF;
 IF NOT (SELECT count(*)=971 AND count(total_tokens)=959 AND sum(total_tokens)=7700211 AND count(*)FILTER(WHERE total_tokens IS NULL)=12 FROM public.behavior76_attempts) THEN RAISE EXCEPTION 'Inclusive accounting mismatch'; END IF;
 IF NOT (SELECT count(*)=197 AND min(n)=1 AND max(n)=197 AND count(total_tokens)=197 AND sum(total_tokens)=1751497 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T195902') THEN RAISE EXCEPTION 'R3 accounting mismatch'; END IF;
 IF NOT (SELECT closed AND admitted=197 FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T195902') THEN RAISE EXCEPTION 'R3 not closed'; END IF;
 IF current_setting('transaction_read_only')<>'on' THEN RAISE EXCEPTION 'Actual read-only comparison required'; END IF;
 RAISE NOTICE 'EXACT_CLOSED_LOCAL_RESPONSE_R3:971 rows equal sealed export; original774 unchanged;12closed windows;959measured/7700211known/12NULL; R3:197admitted/197measured/1751497known/0NULL; noentry198; actual_transaction_read_only=on; no application journal writes';
END $proof$;
ROLLBACK;
