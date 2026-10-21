-- Transport identity is private durable input state, never a payload mode flag.
ALTER TABLE accepted_inputs ADD COLUMN communication_ingress_epoch INT NOT NULL DEFAULT 0 CHECK(communication_ingress_epoch IN(0,1));
ALTER TABLE accepted_inputs ADD COLUMN communication_ingress_json JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(communication_ingress_json)='object');
CREATE FUNCTION butler_immutable_communication_ingress() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.communication_ingress_epoch=1 AND (NEW.communication_ingress_epoch,NEW.communication_ingress_json) IS DISTINCT FROM (OLD.communication_ingress_epoch,OLD.communication_ingress_json) THEN RAISE EXCEPTION 'communication ingress identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_communication_ingress BEFORE UPDATE ON accepted_inputs FOR EACH ROW EXECUTE FUNCTION butler_immutable_communication_ingress();
CREATE OR REPLACE FUNCTION butler_communication_claim_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='running' AND NEW.claim_id IS DISTINCT FROM OLD.claim_id AND
 (NEW.communication_ingress_epoch>COALESCE(NULLIF(current_setting('butler.communication_epoch',TRUE),''),'0')::int OR EXISTS(SELECT 1 FROM communication_admissions WHERE input_id=NEW.input_id AND schema_epoch>COALESCE(NULLIF(current_setting('butler.communication_epoch',TRUE),''),'0')::int))
 THEN RAISE EXCEPTION 'unsupported communication admission epoch'; END IF;
 RETURN NEW;
END $$;
