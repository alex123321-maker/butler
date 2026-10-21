DO $$ BEGIN IF EXISTS(SELECT 1 FROM communication_publications) THEN RAISE EXCEPTION 'communication privacy history must be retained'; END IF; END $$;
DROP TRIGGER communication_message_redaction ON messages;
DROP FUNCTION butler_redact_communication_message();
CREATE OR REPLACE FUNCTION butler_immutable_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'communication publication is immutable'; END $$;
CREATE OR REPLACE FUNCTION butler_immutable_publication_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.publication_id IS DISTINCT FROM OLD.publication_id OR NEW.binding_id IS DISTINCT FROM OLD.binding_id OR NEW.binding_version IS DISTINCT FROM OLD.binding_version OR NEW.recipient_snapshot IS DISTINCT FROM OLD.recipient_snapshot OR NEW.representation_json IS DISTINCT FROM OLD.representation_json THEN RAISE EXCEPTION 'publication delivery identity is immutable'; END IF;
 RETURN NEW;
END $$;
ALTER TABLE communication_interactions DROP COLUMN privacy_redacted;
ALTER TABLE communication_publications DROP COLUMN privacy_redacted,DROP COLUMN redaction_id;
