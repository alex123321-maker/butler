DROP TABLE action_effect_evidence;
DROP FUNCTION reject_effect_evidence_mutation();
DROP TABLE action_effect_dispatches;
ALTER TABLE action_intents DROP COLUMN effect_id;
ALTER TABLE action_intents DROP COLUMN effect_fingerprint;
ALTER TABLE action_intents DROP COLUMN effect_policy;
DROP TABLE action_effects;
