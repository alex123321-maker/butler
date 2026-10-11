-- Private, immutable metadata for the optional guarded native-host route.
-- No browser DATA, password, cookie, extension token or page snapshot is stored.
ALTER TABLE execution_resource_dispatches
 ADD COLUMN single_tab_binding JSONB;

-- Owning requests are finite and pin original authority. Only state may change.
CREATE TABLE guarded_single_tab_requests (
 request_id TEXT PRIMARY KEY,
 approval_id TEXT NOT NULL UNIQUE,
 owner_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 browser_instance TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL CHECK(expires_at>created_at AND expires_at<=created_at+INTERVAL '15 minutes'),
 state TEXT NOT NULL CHECK(state IN ('pending','selected','revoked'))
);
CREATE TABLE guarded_single_tab_selections (
 session_id TEXT PRIMARY KEY,
 request_id TEXT NOT NULL UNIQUE REFERENCES guarded_single_tab_requests(request_id),
 candidate_token TEXT NOT NULL,
 tab_ref TEXT NOT NULL,
 origin TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL
);
CREATE FUNCTION guarded_single_tab_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='guarded_single_tab_requests' AND TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state')
   OR NOT ((OLD.state='pending' AND NEW.state IN ('selected','revoked')) OR (OLD.state='selected' AND NEW.state='revoked') OR OLD.state=NEW.state) THEN
   RAISE EXCEPTION 'guarded selection authority is immutable';
  END IF;
  RETURN NEW;
 END IF;
 RAISE EXCEPTION 'guarded selection history is immutable';
END $$;
CREATE TRIGGER guarded_request_immutable BEFORE UPDATE OR DELETE ON guarded_single_tab_requests FOR EACH ROW EXECUTE FUNCTION guarded_single_tab_immutable();
CREATE TRIGGER guarded_selection_immutable BEFORE UPDATE OR DELETE ON guarded_single_tab_selections FOR EACH ROW EXECUTE FUNCTION guarded_single_tab_immutable();
