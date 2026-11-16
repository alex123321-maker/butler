ALTER TABLE execution_model_attempts DROP CONSTRAINT execution_model_attempts_output_allowance_check;
ALTER TABLE execution_model_attempts ADD CONSTRAINT execution_model_attempts_output_allowance_check
 CHECK (output_allowance > 0 OR (provider = 'antigravity-acp' AND input_allowance = 0 AND output_allowance = 0));
