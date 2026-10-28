-- A rollback of executable code must never erase newer owner decisions/effects.
-- Schema removal is supported only before this subsystem has any evidence.
DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM maintenance_profiles) OR EXISTS(SELECT 1 FROM maintenance_events)
 OR EXISTS(SELECT 1 FROM maintenance_attempts) OR EXISTS(SELECT 1 FROM maintenance_permissions)
 OR EXISTS(SELECT 1 FROM maintenance_observations) THEN
  RAISE EXCEPTION 'nonempty maintenance history cannot be removed' USING ERRCODE='55000';
 END IF;
END
$$;
ALTER TABLE maintenance_targets DROP CONSTRAINT maintenance_unresolved_attempt;
ALTER TABLE maintenance_targets DROP CONSTRAINT maintenance_target_observation;
DROP TABLE maintenance_events,maintenance_observations,maintenance_outcomes,maintenance_attempts,maintenance_permissions,maintenance_reviews,maintenance_stages,maintenance_targets,maintenance_profiles;
DROP FUNCTION butler_maintenance_attempt_transition(),butler_maintenance_permission_transition(),butler_maintenance_target_transition(),butler_maintenance_append_only();
