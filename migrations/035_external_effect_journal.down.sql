DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM action_effects) THEN
  RAISE EXCEPTION 'external effect history exists; downgrade would discard irreversible action evidence';
 END IF;
END $$;
DROP TABLE action_effect_evidence;
DROP FUNCTION reject_effect_evidence_mutation();
DROP TABLE action_effect_dispatches;
ALTER TABLE action_intents DROP COLUMN effect_id;
ALTER TABLE action_intents DROP COLUMN effect_fingerprint;
ALTER TABLE action_intents DROP COLUMN effect_policy;
DROP TABLE action_effects;
