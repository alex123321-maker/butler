DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM execution_model_attempts WHERE output_allowance = 0) THEN
  RAISE EXCEPTION 'cannot discard retained opaque ACP accounting';
 END IF;
END $$;
ALTER TABLE execution_model_attempts DROP CONSTRAINT execution_model_attempts_output_allowance_check;
ALTER TABLE execution_model_attempts ADD CONSTRAINT execution_model_attempts_output_allowance_check CHECK (output_allowance > 0);
