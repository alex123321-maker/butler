DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM communication_telegram_cursors WHERE next_update_id>0) THEN
  RAISE EXCEPTION 'consumed Telegram updates prevent cursor rollback';
 END IF;
END $$;
DROP TABLE communication_telegram_cursors;
