DROP TABLE completion_aux_jobs;
DROP TABLE delivery_outbox;
DROP FUNCTION butler_immutable_notification();
DROP TRIGGER immutable_final_message ON messages;
DROP TRIGGER immutable_final_artifact ON artifacts;
DROP FUNCTION butler_protect_final_components();
DROP TABLE run_final_results;
DROP FUNCTION butler_immutable_final_result();
