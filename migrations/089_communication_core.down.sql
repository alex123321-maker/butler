DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_admissions) OR EXISTS(SELECT 1 FROM communication_publications) OR EXISTS(SELECT 1 FROM communication_bindings) THEN RAISE EXCEPTION 'communication history must be retained; disable admissions and drain with a compatible worker'; END IF;
END $$;
DROP TRIGGER communication_claim_epoch ON accepted_inputs;
DROP FUNCTION butler_communication_claim_epoch();
DROP TRIGGER immutable_publication_delivery ON delivery_outbox;
DROP FUNCTION butler_immutable_publication_delivery();
ALTER TABLE delivery_outbox DROP CONSTRAINT delivery_payload_owner;
DROP INDEX communication_delivery_identity;
ALTER TABLE delivery_outbox DROP COLUMN representation_json, DROP COLUMN recipient_snapshot, DROP COLUMN binding_version, DROP COLUMN binding_id, DROP COLUMN publication_id;
ALTER TABLE delivery_outbox ALTER COLUMN result_id SET NOT NULL;
DROP TABLE communication_events,communication_interactions,communication_publications,communication_admissions,communication_bindings;
DROP FUNCTION butler_immutable_publication();
