CREATE TABLE guarded_single_tab_dispatches (
 request_id TEXT PRIMARY KEY REFERENCES guarded_single_tab_requests(request_id),
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 tool_call_id TEXT NOT NULL,
 effect_id TEXT NOT NULL UNIQUE REFERENCES action_effects(action_id),
 authorization_id TEXT NOT NULL UNIQUE REFERENCES action_authorizations(authorization_id),
 contract_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,tool_call_id)
);
CREATE FUNCTION guarded_tab_dispatch_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'guarded request dispatch history is immutable';
END
$$;
CREATE TRIGGER guarded_dispatch_immutable BEFORE UPDATE OR DELETE ON guarded_single_tab_dispatches FOR EACH ROW EXECUTE FUNCTION guarded_tab_dispatch_immutable();
