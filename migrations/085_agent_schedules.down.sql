DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM scheduled_jobs) OR EXISTS(SELECT 1 FROM schedule_control_receipts) THEN
  RAISE EXCEPTION 'schedule history must be preserved before rollback';
 END IF;
END $$;
DROP FUNCTION butler_task_check_schedule(TEXT);
DROP TABLE schedule_reminder_outbox;
DROP FUNCTION butler_schedule_notice_immutable();
DROP TABLE schedule_control_receipts;
DROP FUNCTION butler_schedule_receipt_immutable();
DROP TABLE scheduled_occurrences;
DROP TABLE scheduled_jobs;
