ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_type_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_format_check;
ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_type_check
        CHECK (artifact_type IN ('assistant_final', 'doctor_report', 'tool_result', 'summary', 'browser_capture')),
    ADD CONSTRAINT artifacts_format_check
        CHECK (content_format IN ('text', 'markdown', 'json', 'image_data_url'));
