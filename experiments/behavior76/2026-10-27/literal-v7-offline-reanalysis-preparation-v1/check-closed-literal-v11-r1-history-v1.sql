\set ON_ERROR_STOP on
CREATE TEMP TABLE checker_literal_current(raw text);
CREATE TEMP TABLE checker_literal_prior(raw text);
CREATE TEMP TABLE checker_literal_prior_windows(raw text);
\copy checker_literal_current(raw) FROM '/src20/.cache/behavior76/native-literal-v11-r1/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_literal_prior(raw) FROM '/src20/.cache/behavior76/native-local-response-v10-r3/behavior76_attempts.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
\copy checker_literal_prior_windows(raw) FROM '/src20/.cache/behavior76/native-local-response-v10-r3/behavior76_windows.jsonl' WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02', ESCAPE E'\x02');
BEGIN READ ONLY;
SET LOCAL statement_timeout='15s';
DO $proof$
DECLARE exact_rows boolean; old_rows boolean; old_windows boolean; new_n bigint; observed bigint; tokens bigint; nulls bigint;
BEGIN
 IF EXISTS(SELECT 1 FROM checker_literal_current GROUP BY raw::jsonb->>'window_id',raw::jsonb->>'n' HAVING count(*)<>1) THEN RAISE EXCEPTION 'Duplicate exported identity'; END IF;
 WITH e AS (SELECT raw::jsonb value FROM checker_literal_current)
 SELECT bool_and(e.value IS NOT NULL AND a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO exact_rows
 FROM e FULL JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT COALESCE(exact_rows,false) THEN RAISE EXCEPTION 'Current exact export differs'; END IF;
 WITH e AS (SELECT raw::jsonb value FROM checker_literal_prior)
 SELECT count(*)=971 AND bool_and(a.window_id IS NOT NULL AND e.value=to_jsonb(a)) INTO old_rows
 FROM e LEFT JOIN public.behavior76_attempts a ON a.window_id=e.value->>'window_id' AND a.n=(e.value->>'n')::integer;
 IF NOT COALESCE(old_rows,false) THEN RAISE EXCEPTION 'Original971 rows changed'; END IF;
 WITH e AS (SELECT raw::jsonb value FROM checker_literal_prior_windows)
 SELECT count(*)=12 AND bool_and(w.id IS NOT NULL AND e.value=to_jsonb(w)) INTO old_windows
 FROM e LEFT JOIN public.behavior76_windows w ON w.id=e.value->>'id';
 IF NOT COALESCE(old_windows,false) THEN RAISE EXCEPTION 'Original12 window records changed'; END IF;
 IF NOT (SELECT count(*)=971 AND count(total_tokens)=959 AND sum(total_tokens)=7700211 AND count(*)FILTER(WHERE total_tokens IS NULL)=12 FROM public.behavior76_attempts WHERE window_id<>'issue76-fixture-literal-20261027T030620') THEN RAISE EXCEPTION 'Prior accounting changed'; END IF;
 SELECT count(*),count(total_tokens),sum(total_tokens),count(*)FILTER(WHERE total_tokens IS NULL) INTO new_n,observed,tokens,nulls FROM public.behavior76_attempts WHERE window_id='issue76-fixture-literal-20261027T030620';
 IF new_n<1 OR new_n>768 OR NOT (SELECT min(n)=1 AND max(n)=new_n AND count(DISTINCT n)=new_n FROM public.behavior76_attempts WHERE window_id='issue76-fixture-literal-20261027T030620') THEN RAISE EXCEPTION 'Literal sequence mismatch'; END IF;
 IF NOT (SELECT closed AND admitted=new_n AND manifest->>'version'='behavior76-literal-supplement-v11' AND (manifest->'literal_plan'->>'arms')::integer=32 AND (manifest->'literal_plan'->>'owner_opportunities')::integer=68 FROM public.behavior76_windows WHERE id='issue76-fixture-literal-20261027T030620') THEN RAISE EXCEPTION 'Literal closure/plan mismatch'; END IF;
 IF NOT (SELECT count(*)=13 AND bool_and(closed) AND sum(admitted)=971+new_n FROM public.behavior76_windows) THEN RAISE EXCEPTION 'Inclusive closure mismatch'; END IF;
 IF current_setting('transaction_read_only')<>'on' THEN RAISE EXCEPTION 'Actual read-only comparison required'; END IF;
 RAISE NOTICE 'LITERAL_R1_ACCOUNTING %',json_build_object('new_entries',new_n,'new_observed',observed,'new_known_tokens',tokens,'new_NULL',nulls,'inclusive_entries',971+new_n,'inclusive_observed',959+observed,'inclusive_known_tokens',7700211+tokens,'inclusive_NULL',12+nulls,'closed_windows',13,'original971_exact',true,'original12_windows_exact',true,'current_export_exact',true,'actual_comparison_read_only',true,'application_journal_writes',0);
END $proof$;
ROLLBACK;
