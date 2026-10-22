DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM accepted_inputs WHERE communication_ingress_epoch>0) THEN RAISE EXCEPTION 'refusing to discard verified Telegram ingress history'; END IF;
END $$;
CREATE OR REPLACE FUNCTION butler_communication_claim_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='running' AND NEW.claim_id IS DISTINCT FROM OLD.claim_id AND EXISTS(SELECT 1 FROM communication_admissions WHERE input_id=NEW.input_id AND schema_epoch>COALESCE(NULLIF(current_setting('butler.communication_epoch',TRUE),''),'0')::int) THEN RAISE EXCEPTION 'unsupported communication admission epoch'; END IF;
 RETURN NEW;
END $$;
DROP TRIGGER immutable_communication_ingress ON accepted_inputs;
DROP FUNCTION butler_immutable_communication_ingress();
ALTER TABLE accepted_inputs DROP COLUMN communication_ingress_json,DROP COLUMN communication_ingress_epoch;
