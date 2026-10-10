CREATE TABLE initiative_digest_outbox (
 notification_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key),
 bucket TIMESTAMPTZ NOT NULL, channel TEXT NOT NULL CHECK(channel IN ('web','telegram')),
 content TEXT NOT NULL, source_ids_json JSONB NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','sending','sent','result_ready','delivery_unknown','failed')),
 attempt_count INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 deadline_at TIMESTAMPTZ NOT NULL, claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ,
 channel_ack TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(session_key,bucket)
);
CREATE TABLE initiative_digest_events (
 initiative_id TEXT NOT NULL REFERENCES initiatives(initiative_id),
 state TEXT NOT NULL CHECK(state IN ('promoted','completed')),
 notification_id TEXT NOT NULL REFERENCES initiative_digest_outbox(notification_id),
 PRIMARY KEY(initiative_id,state)
);
CREATE INDEX initiative_digest_ready ON initiative_digest_outbox(next_attempt_at) WHERE status='pending';
CREATE FUNCTION butler_immutable_initiative_digest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.notification_id IS DISTINCT FROM OLD.notification_id OR NEW.owner_id IS DISTINCT FROM OLD.owner_id OR
    NEW.session_key IS DISTINCT FROM OLD.session_key OR NEW.bucket IS DISTINCT FROM OLD.bucket OR
    NEW.channel IS DISTINCT FROM OLD.channel OR NEW.content IS DISTINCT FROM OLD.content OR
    NEW.source_ids_json IS DISTINCT FROM OLD.source_ids_json OR NEW.deadline_at IS DISTINCT FROM OLD.deadline_at THEN
  RAISE EXCEPTION 'initiative digest payload is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_initiative_digest BEFORE UPDATE ON initiative_digest_outbox FOR EACH ROW EXECUTE FUNCTION butler_immutable_initiative_digest();
