DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_widget_views) THEN
  RAISE EXCEPTION 'accepted widget history requires compatible drain; schema rollback denied';
 END IF;
END $$;
DROP TRIGGER IF EXISTS communication_widget_redact ON communication_interactions;
DROP FUNCTION IF EXISTS communication_widget_redact();
DROP TABLE IF EXISTS communication_widget_projection_ops;
DROP TABLE IF EXISTS communication_widget_actions;
DROP TABLE IF EXISTS communication_widget_views;
