ALTER TABLE execution_notices DROP CONSTRAINT execution_notices_code_check;
ALTER TABLE execution_notices ADD CONSTRAINT execution_notices_code_check CHECK(code IN ('queued','working','waiting_for_user','approval_required','blocked','failed','cancelled','missing_response','finished','queue_blocked'));
ALTER TABLE execution_notices DROP CONSTRAINT execution_notices_reason_check;
ALTER TABLE execution_notices ADD CONSTRAINT execution_notices_reason_check CHECK(reason IN ('','budget','timeout','uncertain_action','source_unavailable','execution_failed','model_unavailable','previous_stopped','previous_waiting'));
