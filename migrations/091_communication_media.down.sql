-- Refuse rollback while retained media (including redacted history) exists.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM artifacts WHERE artifact_type IN ('communication_media','communication_report') OR content_format='private_media_v1') THEN
  RAISE EXCEPTION 'retained communication media prevents rollback';
 END IF;
END $$;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_type_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_type_check CHECK
 (artifact_type IN ('assistant_final','doctor_report','tool_result','summary','browser_capture'));
ALTER TABLE artifacts DROP CONSTRAINT artifacts_format_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_format_check CHECK
 (content_format IN ('text','markdown','json','image_data_url'));
