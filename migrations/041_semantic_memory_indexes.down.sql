-- Refuse discarding activation/proof history, including rollback generations.
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM semantic_index_generations) THEN
        RAISE EXCEPTION 'semantic index history exists; migration rollback refused';
    END IF;
END $$;
DROP TABLE IF EXISTS semantic_index_active;
DROP TABLE IF EXISTS semantic_index_entries;
DROP TABLE IF EXISTS semantic_index_generations;
