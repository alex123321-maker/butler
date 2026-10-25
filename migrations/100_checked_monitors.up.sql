-- Observation state contains no copied source prose. Actual pages remain in
-- the existing deletion-aware tool transcript and publication source graph.
CREATE TABLE schedule_observations (
 occurrence_id TEXT PRIMARY KEY REFERENCES scheduled_occurrences(occurrence_id),
 schedule_id TEXT NOT NULL REFERENCES scheduled_jobs(schedule_id),
 schedule_revision BIGINT NOT NULL,
 input_id TEXT NOT NULL REFERENCES accepted_inputs(input_id),
 owner_id TEXT NOT NULL, run_id TEXT NOT NULL REFERENCES runs(run_id),
 effect_id TEXT UNIQUE NOT NULL, tool_call_id TEXT NOT NULL,
 payload_hash TEXT NOT NULL, event_key_hash TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('confirmed','postponed','pending','unknown')),
 state_hash TEXT NOT NULL, notified BOOLEAN NOT NULL,
 receipt_json JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX schedule_observation_changes ON schedule_observations(schedule_id,schedule_revision,state_hash) WHERE notified;
CREATE TRIGGER schedule_observation_immutable BEFORE UPDATE OR DELETE ON schedule_observations FOR EACH ROW EXECUTE FUNCTION butler_schedule_receipt_immutable();
