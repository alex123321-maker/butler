ALTER TABLE communication_publications ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE,ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE communication_interactions ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE;
CREATE OR REPLACE FUNCTION butler_immutable_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND NEW.payload='{}' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
 AND (to_jsonb(NEW)-ARRAY['payload','privacy_redacted','redaction_id'])=(to_jsonb(OLD)-ARRAY['payload','privacy_redacted','redaction_id']) THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'communication publication is immutable';
END $$;
CREATE OR REPLACE FUNCTION butler_immutable_publication_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.privacy_redacted AND NEW.privacy_redacted AND NEW.representation_json='{}' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
 AND (to_jsonb(NEW)-'representation_json')=(to_jsonb(OLD)-'representation_json') THEN RETURN NEW; END IF;
 IF NEW.publication_id IS DISTINCT FROM OLD.publication_id OR NEW.binding_id IS DISTINCT FROM OLD.binding_id OR NEW.binding_version IS DISTINCT FROM OLD.binding_version OR NEW.recipient_snapshot IS DISTINCT FROM OLD.recipient_snapshot OR NEW.representation_json IS DISTINCT FROM OLD.representation_json THEN RAISE EXCEPTION 'publication delivery identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION butler_redact_communication_message() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id) THEN
  UPDATE communication_publications SET payload='{}',privacy_redacted=TRUE,redaction_id=NEW.redaction_id WHERE publication_id=OLD.message_id AND NOT privacy_redacted;
  UPDATE communication_interactions SET question_json='{}',response_json=NULL,privacy_redacted=TRUE,state=CASE WHEN state='open' THEN 'cancelled' ELSE state END WHERE publication_id=OLD.message_id OR 'response:'||interaction_id=OLD.message_id;
  UPDATE delivery_outbox SET content='',privacy_redacted=TRUE,redaction_id=NEW.redaction_id,status=CASE WHEN status='sending' THEN 'delivery_unknown' WHEN status='pending' THEN 'failed' ELSE status END,error_code=CASE WHEN status IN ('pending','sending') THEN 'source_unavailable' ELSE error_code END,claim_id='',claim_until=NULL WHERE publication_id=OLD.message_id AND NOT privacy_redacted;
  UPDATE delivery_outbox SET representation_json='{}' WHERE publication_id=OLD.message_id AND privacy_redacted AND representation_json<>'{}';
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER communication_message_redaction AFTER UPDATE OF privacy_redacted ON messages FOR EACH ROW EXECUTE FUNCTION butler_redact_communication_message();
