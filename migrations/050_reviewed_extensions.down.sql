DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM extension_versions) OR EXISTS (SELECT 1 FROM extension_probes) THEN
        RAISE EXCEPTION 'refusing to discard durable extension provenance and probe history';
    END IF;
END $$;
DROP TABLE extension_probes;
DROP TABLE extension_task_access;
DROP TABLE extension_events;
DROP TABLE extension_installations;
DROP TABLE extension_reviews;
DROP TABLE extension_versions;
DROP FUNCTION reject_extension_history_update();
