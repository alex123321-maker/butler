BEGIN READ ONLY;
SELECT json_build_object(
 'version','butler-structural-json-pg-witness-v1',
 'window',(SELECT row_to_json(w) FROM fixture_structural_json_window w),
 'attempts',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY ordinal) FROM fixture_structural_json_attempts a),'[]'::json),
 'retention',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY ordinal) FROM fixture_structural_json_retention r),'[]'::json)
);
COMMIT;
