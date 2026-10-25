-- Maintenance authority is separate from task/tool permissions. New targets
-- start stopped. These records do not add any model or runtime capability.
CREATE TABLE maintenance_profiles (
 profile_sha256 TEXT PRIMARY KEY CHECK(profile_sha256 ~ '^[a-f0-9]{64}$'),
 owner_id TEXT NOT NULL CHECK(length(owner_id) BETWEEN 1 AND 160),
 target_id TEXT NOT NULL CHECK(target_id ~ '^[a-z][a-z0-9_-]{0,62}$'),
 raw BYTEA NOT NULL CHECK(octet_length(raw) BETWEEN 1 AND 1048576),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(encode(sha256(raw),'hex')=profile_sha256),
 UNIQUE(profile_sha256,owner_id,target_id)
);
CREATE TABLE maintenance_targets (
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 profile_sha256 TEXT NOT NULL,
 stopped BOOLEAN NOT NULL DEFAULT TRUE,
 epoch BIGINT NOT NULL DEFAULT 1 CHECK(epoch>0),
 next_fence BIGINT NOT NULL DEFAULT 1 CHECK(next_fence>0),
 artifact_sha256 TEXT CHECK(artifact_sha256 ~ '^[a-f0-9]{64}$'),
 inventory_sha256 TEXT CHECK(inventory_sha256 ~ '^[a-f0-9]{64}$'),
 observation_sha256 TEXT CHECK(observation_sha256 ~ '^[a-f0-9]{64}$'),
 observed_at TIMESTAMPTZ,
 unresolved_attempt TEXT,
 PRIMARY KEY(owner_id,target_id),
 FOREIGN KEY(profile_sha256,owner_id,target_id) REFERENCES maintenance_profiles(profile_sha256,owner_id,target_id),
 CHECK((artifact_sha256 IS NULL)=(inventory_sha256 IS NULL)),
 CHECK((observation_sha256 IS NULL)=(observed_at IS NULL))
);
CREATE TABLE maintenance_stages (
 stage_id TEXT PRIMARY KEY CHECK(stage_id ~ '^mstage_[a-f0-9]{64}$'),
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 profile_sha256 TEXT NOT NULL,
 binding BYTEA NOT NULL CHECK(octet_length(binding) BETWEEN 1 AND 8192),
 source_inventory BYTEA NOT NULL CHECK(octet_length(source_inventory) BETWEEN 1 AND 33554432),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(profile_sha256,owner_id,target_id) REFERENCES maintenance_profiles(profile_sha256,owner_id,target_id),
 CHECK('mstage_'||encode(sha256(binding),'hex')=stage_id),
 UNIQUE(stage_id,owner_id,target_id,profile_sha256)
);
CREATE TABLE maintenance_reviews (
 review_sha256 TEXT PRIMARY KEY CHECK(review_sha256 ~ '^[a-f0-9]{64}$'),
 stage_id TEXT NOT NULL REFERENCES maintenance_stages(stage_id),
 raw BYTEA NOT NULL CHECK(octet_length(raw) BETWEEN 1 AND 1048576),
 signature BYTEA NOT NULL CHECK(octet_length(signature)=64),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(encode(sha256(raw),'hex')=review_sha256),
 UNIQUE(review_sha256,stage_id)
);
CREATE TABLE maintenance_permissions (
 permission_id TEXT PRIMARY KEY CHECK(permission_id ~ '^mpermit_[a-f0-9]{64}$'),
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 profile_sha256 TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('ordinary_mandate','exact_protected','exact_rollback')),
 epoch BIGINT NOT NULL CHECK(epoch>0),
 nonce TEXT NOT NULL CHECK(nonce ~ '^[a-f0-9]{64}$'),
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 raw BYTEA NOT NULL CHECK(octet_length(raw) BETWEEN 1 AND 65536),
 signature BYTEA NOT NULL CHECK(octet_length(signature)=64),
 limit_claims INTEGER NOT NULL CHECK(limit_claims BETWEEN 1 AND 10),
 used_claims INTEGER NOT NULL DEFAULT 0 CHECK(used_claims>=0 AND used_claims<=limit_claims),
 expires_at TIMESTAMPTZ NOT NULL,
 revoked BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(profile_sha256,owner_id,target_id) REFERENCES maintenance_profiles(profile_sha256,owner_id,target_id),
 CHECK('mpermit_'||encode(sha256(raw),'hex')=permission_id),
 CHECK(((convert_from(raw,'UTF8')::jsonb->>'epoch')::bigint) IS NOT DISTINCT FROM epoch),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'nonce') IS NOT DISTINCT FROM nonce),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'request_sha256') IS NOT DISTINCT FROM request_sha256),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'kind') IS NOT DISTINCT FROM kind),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'owner_id') IS NOT DISTINCT FROM owner_id),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'target_id') IS NOT DISTINCT FROM target_id),
 CHECK((convert_from(raw,'UTF8')::jsonb->>'profile_sha256') IS NOT DISTINCT FROM profile_sha256),
 CHECK(((convert_from(raw,'UTF8')::jsonb->>'max_claims')::bigint) IS NOT DISTINCT FROM limit_claims),
 CHECK(((convert_from(raw,'UTF8')::jsonb->>'expires_at')::timestamptz) IS NOT DISTINCT FROM expires_at),
 CHECK(expires_at>created_at AND expires_at<=created_at+INTERVAL '24 hours'),
 UNIQUE(permission_id,owner_id,target_id,profile_sha256),
 UNIQUE(owner_id,target_id,nonce)
);
CREATE TABLE maintenance_attempts (
 attempt_id TEXT PRIMARY KEY CHECK(attempt_id ~ '^mdeploy_[a-f0-9]{64}$'),
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 profile_sha256 TEXT NOT NULL,
 stage_id TEXT NOT NULL,
 review_sha256 TEXT NOT NULL,
 permission_id TEXT NOT NULL,
 purpose TEXT NOT NULL CHECK(purpose IN ('promote','rollback')),
 fence BIGINT NOT NULL CHECK(fence>0),
 epoch BIGINT NOT NULL CHECK(epoch>0),
 base_artifact_sha256 TEXT NOT NULL CHECK(base_artifact_sha256 ~ '^[a-f0-9]{64}$'),
 artifact_sha256 TEXT NOT NULL CHECK(artifact_sha256 ~ '^[a-f0-9]{64}$'),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','dispatching','completed','failed','unknown','stopped')),
 claim BYTEA NOT NULL CHECK(octet_length(claim) BETWEEN 1 AND 16384),
 signature BYTEA NOT NULL CHECK(octet_length(signature)=64),
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 dispatched_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 FOREIGN KEY(stage_id,owner_id,target_id,profile_sha256) REFERENCES maintenance_stages(stage_id,owner_id,target_id,profile_sha256),
 FOREIGN KEY(review_sha256,stage_id) REFERENCES maintenance_reviews(review_sha256,stage_id),
 FOREIGN KEY(permission_id,owner_id,target_id,profile_sha256) REFERENCES maintenance_permissions(permission_id,owner_id,target_id,profile_sha256),
 UNIQUE(attempt_id,owner_id,target_id,profile_sha256),
 UNIQUE(owner_id,target_id,fence),
 CHECK(expires_at>created_at AND expires_at<=created_at+INTERVAL '10 minutes'),
 CHECK((dispatched_at IS NOT NULL)=(state IN ('dispatching','completed','failed','unknown'))),
 CHECK((ended_at IS NOT NULL)=(state IN ('completed','failed','unknown','stopped')))
);
ALTER TABLE maintenance_targets ADD CONSTRAINT maintenance_unresolved_attempt
 FOREIGN KEY(unresolved_attempt,owner_id,target_id,profile_sha256) REFERENCES maintenance_attempts(attempt_id,owner_id,target_id,profile_sha256);
CREATE TABLE maintenance_outcomes (
 attempt_id TEXT PRIMARY KEY REFERENCES maintenance_attempts(attempt_id),
 outcome_sha256 TEXT NOT NULL CHECK(outcome_sha256 ~ '^[a-f0-9]{64}$'),
 raw BYTEA NOT NULL CHECK(octet_length(raw) BETWEEN 1 AND 16384),
 signature BYTEA NOT NULL CHECK(octet_length(signature)=64),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(encode(sha256(raw),'hex')=outcome_sha256)
);
CREATE UNIQUE INDEX maintenance_one_unended_attempt ON maintenance_attempts(owner_id,target_id)
 WHERE state IN ('pending','dispatching');
CREATE TABLE maintenance_observations (
 observation_sha256 TEXT PRIMARY KEY CHECK(observation_sha256 ~ '^[a-f0-9]{64}$'),
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 profile_sha256 TEXT NOT NULL,
 nonce TEXT NOT NULL CHECK(nonce ~ '^[a-f0-9]{64}$'),
 raw BYTEA NOT NULL CHECK(octet_length(raw) BETWEEN 1 AND 16384),
 signature BYTEA NOT NULL CHECK(octet_length(signature)=64),
 observed_at TIMESTAMPTZ NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(encode(sha256(raw),'hex')=observation_sha256),
 CHECK(observed_at<=recorded_at),
 FOREIGN KEY(profile_sha256,owner_id,target_id) REFERENCES maintenance_profiles(profile_sha256,owner_id,target_id),
 UNIQUE(owner_id,target_id,nonce),
 UNIQUE(observation_sha256,owner_id,target_id,profile_sha256)
);
ALTER TABLE maintenance_targets ADD CONSTRAINT maintenance_target_observation
 FOREIGN KEY(observation_sha256,owner_id,target_id,profile_sha256) REFERENCES maintenance_observations(observation_sha256,owner_id,target_id,profile_sha256);
CREATE TABLE maintenance_events (
 event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id TEXT NOT NULL,
 target_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('profile','stop','resume','observation','stage','review','permission','revoke','claim','dispatch','outcome','reconcile')),
 reference TEXT NOT NULL CHECK(length(reference) BETWEEN 1 AND 200),
 epoch BIGINT NOT NULL CHECK(epoch>0),
 evidence_sha256 TEXT CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(owner_id,target_id) REFERENCES maintenance_targets(owner_id,target_id)
);
CREATE FUNCTION butler_maintenance_append_only() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'immutable maintenance evidence' USING ERRCODE='55000';
END
$$;
CREATE TRIGGER maintenance_profiles_immutable BEFORE UPDATE OR DELETE ON maintenance_profiles
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE TRIGGER maintenance_stages_immutable BEFORE UPDATE OR DELETE ON maintenance_stages
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE TRIGGER maintenance_reviews_immutable BEFORE UPDATE OR DELETE ON maintenance_reviews
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE TRIGGER maintenance_events_immutable BEFORE UPDATE OR DELETE ON maintenance_events
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE TRIGGER maintenance_observations_immutable BEFORE UPDATE OR DELETE ON maintenance_observations
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE TRIGGER maintenance_outcomes_immutable BEFORE UPDATE OR DELETE ON maintenance_outcomes
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_append_only();
CREATE FUNCTION butler_maintenance_permission_transition() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'used_claims'-'revoked') IS DISTINCT FROM (to_jsonb(OLD)-'used_claims'-'revoked')
 OR NEW.used_claims<OLD.used_claims OR NEW.used_claims>OLD.used_claims+1 OR (OLD.revoked AND NOT NEW.revoked)
 OR (OLD.revoked AND NEW.used_claims<>OLD.used_claims) THEN
  RAISE EXCEPTION 'invalid maintenance permission mutation' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END
$$;
CREATE TRIGGER maintenance_permission_guard BEFORE UPDATE OR DELETE ON maintenance_permissions
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_permission_transition();
CREATE FUNCTION butler_maintenance_attempt_transition() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'state'-'dispatched_at'-'ended_at') IS DISTINCT FROM (to_jsonb(OLD)-'state'-'dispatched_at'-'ended_at')
 OR NOT ((OLD.state='pending' AND NEW.state IN ('dispatching','stopped'))
 OR (OLD.state='dispatching' AND NEW.state IN ('completed','failed','unknown'))) THEN
  RAISE EXCEPTION 'maintenance attempt cannot replay or rewrite history' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END
$$;
CREATE TRIGGER maintenance_attempt_guard BEFORE UPDATE OR DELETE ON maintenance_attempts
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_attempt_transition();
CREATE FUNCTION butler_maintenance_target_transition() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.owner_id<>OLD.owner_id OR NEW.target_id<>OLD.target_id
 OR NEW.epoch<OLD.epoch OR NEW.epoch>OLD.epoch+1
 OR NEW.next_fence<OLD.next_fence OR NEW.next_fence>OLD.next_fence+1
 OR ((NEW.stopped<>OLD.stopped OR NEW.profile_sha256<>OLD.profile_sha256) AND NEW.epoch<>OLD.epoch+1)
 OR (NEW.profile_sha256<>OLD.profile_sha256 AND NOT NEW.stopped) THEN
  RAISE EXCEPTION 'maintenance target cannot lower its fence or stop epoch' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END
$$;
CREATE TRIGGER maintenance_target_guard BEFORE UPDATE OR DELETE ON maintenance_targets
 FOR EACH ROW EXECUTE FUNCTION butler_maintenance_target_transition();
