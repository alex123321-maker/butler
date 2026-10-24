-- Durable owner ceiling also fences older epoch-1 writers. No credentials or
-- model arguments can set this policy; the private operator composition owns it.
CREATE TABLE communication_owner_limits (
 owner_id text PRIMARY KEY,
 max_open_interactions integer NOT NULL DEFAULT 16 CHECK(max_open_interactions BETWEEN 1 AND 64),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX communication_open_owner ON communication_interactions(owner_id) WHERE state='open';
CREATE FUNCTION butler_communication_open_limit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ceiling integer; outstanding bigint;
BEGIN
 IF NEW.state<>'open' THEN RETURN NEW; END IF;
 IF TG_OP='UPDATE' AND OLD.state='open' AND OLD.owner_id=NEW.owner_id THEN RETURN NEW; END IF;
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'communication question admission requires read committed';
 END IF;
 INSERT INTO communication_owner_limits(owner_id) VALUES(NEW.owner_id) ON CONFLICT DO NOTHING;
 SELECT max_open_interactions INTO STRICT ceiling FROM communication_owner_limits WHERE owner_id=NEW.owner_id FOR UPDATE;
 SELECT count(*) INTO outstanding FROM communication_interactions WHERE owner_id=NEW.owner_id AND state='open';
 IF outstanding>=ceiling THEN RAISE EXCEPTION 'communication owner question limit reached'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER communication_open_limit BEFORE INSERT OR UPDATE OF state,owner_id ON communication_interactions
 FOR EACH ROW EXECUTE FUNCTION butler_communication_open_limit();
