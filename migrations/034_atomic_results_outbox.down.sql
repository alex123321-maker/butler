DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM run_final_results) THEN
  RAISE EXCEPTION 'durable final results exist; downgrade would discard result and delivery history';
 END IF;
END $$;
DROP TABLE completion_aux_jobs;
DROP TABLE delivery_outbox;
DROP FUNCTION butler_immutable_notification();
DROP TRIGGER immutable_final_message ON messages;
DROP TRIGGER immutable_final_artifact ON artifacts;
DROP FUNCTION butler_protect_final_components();
DROP TABLE run_final_results;
DROP FUNCTION butler_immutable_final_result();
