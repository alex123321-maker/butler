-- A restart/worker lease is not owner authority. Bind the actual executor's
-- opened directory inode before any physical effect; old missing bindings deny.
CREATE TABLE code_workspace_executor_bindings (
 ticket_id TEXT PRIMARY KEY REFERENCES code_workspace_dispatches(ticket_id),
 root_identity_hash TEXT NOT NULL CHECK(root_identity_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER code_workspace_executor_bindings_immutable BEFORE UPDATE OR DELETE ON code_workspace_executor_bindings
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_authorization_immutable();

CREATE FUNCTION butler_workspace_dispatch_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'workspace dispatch history is immutable'; END IF;
 IF ROW(NEW.ticket_id,NEW.authorization_id,NEW.effect_id,NEW.resource_id,NEW.resource_lease_id,NEW.resource_version,NEW.resource_fence,NEW.deadline,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.ticket_id,OLD.authorization_id,OLD.effect_id,OLD.resource_id,OLD.resource_lease_id,OLD.resource_version,OLD.resource_fence,OLD.deadline,OLD.created_at)
 OR NOT ((OLD.phase='prepared' AND NEW.phase='running' AND NEW.finished_at IS NULL AND EXISTS(SELECT 1 FROM code_workspace_executor_bindings WHERE ticket_id=OLD.ticket_id))
 OR (OLD.phase='running' AND NEW.phase IN ('completed','unknown') AND NEW.finished_at IS NOT NULL)) THEN
  RAISE EXCEPTION 'workspace dispatch binding is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER code_workspace_dispatches_immutable BEFORE UPDATE OR DELETE ON code_workspace_dispatches
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_dispatch_immutable();

CREATE TABLE code_workspace_recoveries (
 recovery_id TEXT PRIMARY KEY CHECK(recovery_id ~ '^[a-f0-9]{32}$'),
 owner_id TEXT NOT NULL,
 channel TEXT NOT NULL CHECK(channel IN ('web','telegram')),
 delivery_id TEXT NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 workspace_id TEXT NOT NULL CHECK(workspace_id ~ '^[a-f0-9]{32}$'),
 ticket_id TEXT NOT NULL REFERENCES code_workspace_executor_bindings(ticket_id),
 resource_version BIGINT NOT NULL CHECK(resource_version>0),
 workspace_generation BIGINT NOT NULL CHECK(workspace_generation>=0),
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 previous_recovery_id TEXT NOT NULL DEFAULT '',
 choice TEXT NOT NULL DEFAULT 'discard' CHECK(choice='discard'),
 phase TEXT NOT NULL DEFAULT 'requested' CHECK(phase IN ('requested','erasing','discarded','superseded')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 attempted_at TIMESTAMPTZ,
 finished_at TIMESTAMPTZ,
 UNIQUE(owner_id,channel,delivery_id)
);
CREATE UNIQUE INDEX code_workspace_recovery_pending ON code_workspace_recoveries(workspace_id) WHERE phase IN ('requested','erasing');
CREATE FUNCTION butler_workspace_recovery_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'workspace recovery history is immutable'; END IF;
 IF ROW(NEW.recovery_id,NEW.owner_id,NEW.channel,NEW.delivery_id,NEW.request_hash,NEW.workspace_id,NEW.ticket_id,NEW.resource_version,NEW.workspace_generation,NEW.task_revision,NEW.constraints_revision,NEW.previous_recovery_id,NEW.choice,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.recovery_id,OLD.owner_id,OLD.channel,OLD.delivery_id,OLD.request_hash,OLD.workspace_id,OLD.ticket_id,OLD.resource_version,OLD.workspace_generation,OLD.task_revision,OLD.constraints_revision,OLD.previous_recovery_id,OLD.choice,OLD.created_at)
 OR NOT ((OLD.phase='requested' AND NEW.phase='erasing' AND OLD.attempted_at IS NULL AND NEW.attempted_at IS NOT NULL AND NEW.finished_at IS NULL)
 OR (OLD.phase='requested' AND NEW.phase='superseded' AND OLD.attempted_at IS NULL AND NEW.attempted_at IS NULL AND NEW.finished_at IS NOT NULL
 AND EXISTS(SELECT 1 FROM code_workspace_dispatches d JOIN code_workspace_authorizations a USING(authorization_id)
 JOIN tasks t ON t.task_id=a.source_context->>'task_id' JOIN execution_resources r ON r.resource_id=d.resource_id
 LEFT JOIN code_workspaces w ON w.workspace_id=a.workspace_id WHERE d.ticket_id=OLD.ticket_id
 AND r.version=OLD.resource_version AND COALESCE(w.generation,0)=OLD.workspace_generation
 AND (t.revision<>OLD.task_revision OR t.constraints_revision<>OLD.constraints_revision)))
 OR (OLD.phase='erasing' AND NEW.phase IN ('discarded','superseded') AND NEW.attempted_at=OLD.attempted_at AND NEW.finished_at IS NOT NULL)) THEN
  RAISE EXCEPTION 'workspace recovery decision or attempt is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER code_workspace_recoveries_immutable BEFORE UPDATE OR DELETE ON code_workspace_recoveries
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_recovery_immutable();
CREATE TABLE code_workspace_erasure_receipts (
 recovery_id TEXT PRIMARY KEY REFERENCES code_workspace_recoveries(recovery_id),
 observation_hash TEXT NOT NULL CHECK(observation_hash ~ '^[a-f0-9]{64}$'),
 root_identity_hash TEXT NOT NULL CHECK(root_identity_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER code_workspace_erasure_receipts_immutable BEFORE UPDATE OR DELETE ON code_workspace_erasure_receipts
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_authorization_immutable();
