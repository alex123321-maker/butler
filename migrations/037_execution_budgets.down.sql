DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_budget_reservations)
 OR EXISTS(SELECT 1 FROM execution_model_attempts)
 OR EXISTS(SELECT 1 FROM execution_loop_observations)
 OR EXISTS(SELECT 1 FROM execution_budget_scopes)
 THEN RAISE EXCEPTION 'cannot roll back durable execution budget history'; END IF;
END $$;
DROP TABLE execution_loop_observations;
DROP TABLE execution_model_attempts;
DROP TABLE execution_budget_reservations;
DROP TABLE execution_budget_scopes;
