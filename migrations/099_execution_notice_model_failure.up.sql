ALTER TABLE execution_notices DROP CONSTRAINT execution_notices_reason_check;
ALTER TABLE execution_notices ADD CONSTRAINT execution_notices_reason_check CHECK(reason IN ('','budget','timeout','uncertain_action','source_unavailable','execution_failed','model_unavailable'));
