-- Retain acknowledged delivery history: never delete/relabel new notices to
-- force them through an older vocabulary during rollback.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_notices WHERE code='queue_blocked' OR reason IN ('previous_stopped','previous_waiting')) THEN
  RAISE EXCEPTION 'queue dependency notices require a compatible schema';
 END IF;
END $$;
ALTER TABLE execution_notices DROP CONSTRAINT execution_notices_code_check;
ALTER TABLE execution_notices ADD CONSTRAINT execution_notices_code_check CHECK(code IN ('queued','working','waiting_for_user','approval_required','blocked','failed','cancelled','missing_response','finished'));
ALTER TABLE execution_notices DROP CONSTRAINT execution_notices_reason_check;
ALTER TABLE execution_notices ADD CONSTRAINT execution_notices_reason_check CHECK(reason IN ('','budget','timeout','uncertain_action','source_unavailable','execution_failed','model_unavailable'));
