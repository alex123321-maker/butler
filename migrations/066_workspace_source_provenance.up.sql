-- Content-free cumulative provenance for prepared workspace artifacts. Actual
-- derived DATA sources retain the original graph and task scope in Catalog.
CREATE TABLE code_workspace_sources (
 workspace_id TEXT NOT NULL REFERENCES code_workspaces(workspace_id),
 sequence BIGINT NOT NULL CHECK(sequence>0),
 lease_id TEXT NOT NULL UNIQUE REFERENCES execution_resource_leases(lease_id),
 source_id TEXT NOT NULL UNIQUE REFERENCES memory_sources(source_id),
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 access_revision BIGINT NOT NULL CHECK(access_revision>0),
 content_hash TEXT NOT NULL CHECK(content_hash ~ '^[a-f0-9]{64}$'),
 observation_hash TEXT NOT NULL CHECK(observation_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(workspace_id,sequence),
 UNIQUE(workspace_id,source_id)
);
CREATE FUNCTION butler_workspace_source_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'workspace provenance is immutable'; END;
$$;
CREATE TRIGGER code_workspace_sources_immutable BEFORE UPDATE OR DELETE ON code_workspace_sources
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_source_immutable();

CREATE TABLE code_workspace_deletions (
 workspace_id TEXT PRIMARY KEY REFERENCES code_workspaces(workspace_id),
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','done')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 completed_at TIMESTAMPTZ,
 FOREIGN KEY(workspace_id,source_id) REFERENCES code_workspace_sources(workspace_id,source_id)
);
-- Catalog deletion holds the original session/task/source locks. Blocking and
-- queueing these physical copies is part of that same deletion transaction.
CREATE FUNCTION butler_workspace_source_deleted() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status IN ('deleted','revoked') AND OLD.status IS DISTINCT FROM NEW.status THEN
  INSERT INTO code_workspace_deletions(workspace_id,source_id)
   SELECT workspace_id,NEW.source_id FROM code_workspace_sources WHERE source_id=NEW.source_id
   ON CONFLICT(workspace_id) DO NOTHING;
  UPDATE code_workspaces w SET state='blocked',blocked_reason='source_deleted',generation=generation+1
   WHERE w.state='active' AND EXISTS(SELECT 1 FROM code_workspace_sources p WHERE p.workspace_id=w.workspace_id AND p.source_id=NEW.source_id);
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER code_workspace_source_deleted AFTER UPDATE OF status ON memory_sources
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_source_deleted();
