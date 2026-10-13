BEGIN READ ONLY;
SELECT json_build_object(
 'version','butler-schema-dialect-pg-witness-v1',
 'window',(SELECT row_to_json(w) FROM fixture_schema_dialect_window w),
 'attempts',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY ordinal) FROM fixture_schema_dialect_attempts a),'[]'::json),
 'retention',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY ordinal) FROM fixture_schema_dialect_retention r),'[]'::json)
);
COMMIT;
