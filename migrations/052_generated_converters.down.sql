DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM converter_needs) OR EXISTS(SELECT 1 FROM converter_events) THEN
  RAISE EXCEPTION 'refusing to remove durable converter need/package/use history';
 END IF;
END $$;
DROP TABLE converter_events,converter_usages,converter_trials,converter_validations,converter_states;
ALTER TABLE converter_catalog DROP CONSTRAINT converter_catalog_owner_id_active_digest_fkey;
DROP TABLE converter_versions,converter_catalog,converter_needs;
DROP FUNCTION converter_immutable_record();
