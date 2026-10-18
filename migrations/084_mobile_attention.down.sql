DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mobile_messages) OR EXISTS(SELECT 1 FROM mobile_agent_access) THEN
  RAISE EXCEPTION 'mobile attention history exists; disable automation and retain history before rollback';
 END IF;
END $$;
DROP TABLE mobile_noise_rules;
DROP TABLE mobile_messages;
DROP TABLE mobile_attention_batches;
DROP TABLE mobile_agent_access;
ALTER TABLE mobile_events DROP COLUMN messages_indexed;
DROP INDEX mobile_one_source;
