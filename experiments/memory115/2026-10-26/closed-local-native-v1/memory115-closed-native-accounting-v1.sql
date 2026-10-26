\set ON_ERROR_STOP on
BEGIN READ ONLY;
DO $proof$
BEGIN
 IF NOT (SELECT count(*)=1 AND bool_and(closed) AND sum(used)=19 FROM experience_native_v1.experience_native_windows WHERE window_id='dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099') THEN RAISE EXCEPTION 'Native window not exactly closed19'; END IF;
 IF NOT (SELECT count(*)=19 AND count(usage_json)=19 AND sum((usage_json->>'TotalTokens')::bigint)=133011 AND max(sequence)=19 FROM experience_native_v1.experience_native_entries WHERE window_id='dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099') THEN RAISE EXCEPTION 'Native accounting differs'; END IF;
 RAISE NOTICE 'CLOSED_LOCAL115: one closed window,19native entries/19measured/133011known full tokens/no entry20';
END $proof$;
ROLLBACK;
