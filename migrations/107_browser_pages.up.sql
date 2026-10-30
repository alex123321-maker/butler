CREATE TABLE server_browser_pages (
 browser_session_id TEXT NOT NULL REFERENCES server_browser_sessions(browser_session_id),
 page_id TEXT NOT NULL CHECK(page_id ~ '^[0-9a-f]{32}$'),
 opener_page_id TEXT,
 document_id TEXT NOT NULL DEFAULT '' CHECK(document_id='' OR document_id ~ '^[0-9a-f]{32}$'),
 observation_version BIGINT NOT NULL DEFAULT 0 CHECK(observation_version>=0),
 observed_epoch BIGINT NOT NULL DEFAULT -1 CHECK(observed_epoch>=-1),
 observed_url TEXT NOT NULL DEFAULT '' CHECK(length(observed_url)<=2048 AND observed_url !~ '[?#]'),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','ready','closed','lost')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(browser_session_id,page_id),
 FOREIGN KEY(browser_session_id,opener_page_id) REFERENCES server_browser_pages(browser_session_id,page_id),
 CHECK(opener_page_id IS NULL OR opener_page_id<>page_id),
 CHECK(state<>'pending' OR (document_id='' AND observation_version=0 AND observed_epoch=-1 AND observed_url='')),
 CHECK(state<>'ready' OR (document_id<>'' AND observation_version>0 AND observed_epoch>=0 AND observed_url<>''))
);

CREATE FUNCTION retain_server_browser_page() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'UPDATE' THEN RAISE EXCEPTION 'browser page identity must be retained'; END IF;
 IF OLD.state IN ('closed','lost') OR NEW.browser_session_id<>OLD.browser_session_id
  OR NEW.page_id<>OLD.page_id OR NEW.opener_page_id IS DISTINCT FROM OLD.opener_page_id
  OR NEW.created_at<>OLD.created_at OR NEW.observation_version<>OLD.observation_version+1
  OR NEW.state='pending' THEN
  RAISE EXCEPTION 'invalid browser page transition';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER retain_browser_page_update BEFORE UPDATE ON server_browser_pages
 FOR EACH ROW EXECUTE FUNCTION retain_server_browser_page();
CREATE TRIGGER retain_browser_page_delete BEFORE DELETE OR TRUNCATE ON server_browser_pages
 FOR EACH STATEMENT EXECUTE FUNCTION retain_server_browser_page();
