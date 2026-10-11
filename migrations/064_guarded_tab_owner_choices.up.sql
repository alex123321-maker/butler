-- Frozen sanitized display metadata. No native tab/host reference or page DATA.
CREATE TABLE guarded_single_tab_choices (
 request_id TEXT NOT NULL REFERENCES guarded_single_tab_requests(request_id),
 candidate_token TEXT NOT NULL CHECK(candidate_token ~ '^[a-f0-9]{48}$'),
 origin TEXT NOT NULL CHECK(length(origin) BETWEEN 1 AND 512),
 ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 1 AND 64),
 active_at_request BOOLEAN NOT NULL,
 PRIMARY KEY(request_id,candidate_token),
 UNIQUE(request_id,ordinal)
);
CREATE TRIGGER guarded_choice_immutable BEFORE UPDATE OR DELETE ON guarded_single_tab_choices
 FOR EACH ROW EXECUTE FUNCTION guarded_single_tab_immutable();
