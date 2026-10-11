DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM skill_versions) OR EXISTS(SELECT 1 FROM skill_usages) THEN
  RAISE EXCEPTION 'refuse rollback: reviewed skill provenance must be retained';
 END IF;
END $$;
DROP TABLE skill_correction_grants,skill_usages,skill_events,skill_validations,skill_catalog,skill_states,skill_versions;
DROP FUNCTION reject_skill_history_update();
