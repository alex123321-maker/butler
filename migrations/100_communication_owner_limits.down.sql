DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_owner_limits) OR EXISTS(SELECT 1 FROM communication_admissions) THEN
  RAISE EXCEPTION 'accepted communication policy/history requires compatible drain; schema rollback denied';
 END IF;
END $$;
DROP TRIGGER communication_open_limit ON communication_interactions;
DROP FUNCTION butler_communication_open_limit();
DROP INDEX communication_open_owner;
DROP TABLE communication_owner_limits;
