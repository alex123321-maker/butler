-- Channel projections and drafts are DATA. Existing Input/interaction owns resume.
CREATE TABLE communication_widget_views (
 interaction_id text PRIMARY KEY REFERENCES communication_interactions ON DELETE CASCADE,
 callback_token text NOT NULL UNIQUE CHECK (length(callback_token)=24),
 draft_version bigint NOT NULL DEFAULT 1 CHECK (draft_version>0),
 draft_json jsonb NOT NULL DEFAULT '{"option_ids":[]}'
);
CREATE TABLE communication_widget_actions (
 interaction_id text NOT NULL REFERENCES communication_interactions ON DELETE CASCADE,
 delivery_id text NOT NULL,
 payload_hash text NOT NULL,
 receipt jsonb NOT NULL,
 PRIMARY KEY(interaction_id,delivery_id)
);
-- Journal for edits of known ACKs; never creates or retries a publication send.
CREATE TABLE communication_widget_projection_ops (
 notification_id text NOT NULL REFERENCES delivery_outbox ON DELETE CASCADE,
 revision text NOT NULL,
 status text NOT NULL CHECK(status IN ('sending','sent','failed','delivery_unknown')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(notification_id,revision)
);

CREATE FUNCTION communication_widget_redact() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.privacy_redacted THEN
  UPDATE communication_widget_views SET draft_json='{}' WHERE interaction_id=NEW.interaction_id;
  DELETE FROM communication_widget_actions WHERE interaction_id=NEW.interaction_id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER communication_widget_redact AFTER UPDATE OF privacy_redacted ON communication_interactions
 FOR EACH ROW EXECUTE FUNCTION communication_widget_redact();
