-- References and technical proof only; no child narrative or generated body.
CREATE TABLE converter_developer_returns (
 child_task_id TEXT PRIMARY KEY REFERENCES converter_developer_snapshots(task_id),
 owner_id TEXT NOT NULL,
 parent_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 result_id TEXT NOT NULL REFERENCES task_results(result_id),
 child_run_id TEXT NOT NULL REFERENCES runs(run_id),
 digest TEXT NOT NULL,
 projection_hash TEXT NOT NULL CHECK(length(projection_hash)=64),
 projection_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(owner_id,digest) REFERENCES converter_versions(owner_id,digest)
);
CREATE TRIGGER converter_return_immutable BEFORE UPDATE OR DELETE ON converter_developer_returns
 FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();
