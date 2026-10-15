DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_retrieval_plans) THEN
  RAISE EXCEPTION 'retained retrieval evidence prevents destructive rollback';
 END IF;
END $$;
DROP TABLE memory_retrieval_opened;
DROP TABLE memory_retrieval_operations;
DROP TABLE memory_retrieval_plans;
