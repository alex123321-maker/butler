DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM initiative_ticks) OR EXISTS(SELECT 1 FROM initiative_events) OR EXISTS(SELECT 1 FROM initiatives)
    OR EXISTS(SELECT 1 FROM initiative_sources) OR EXISTS(SELECT 1 FROM initiative_schedules)
    OR EXISTS(SELECT 1 FROM initiative_config_events) OR EXISTS(SELECT 1 FROM initiative_attention_days) THEN
  RAISE EXCEPTION 'initiative observation history exists; downgrade would discard processed events';
 END IF;
END $$;
DROP TABLE initiative_attention_days;
DROP TABLE initiative_config_events;
DROP TABLE initiative_events;
DROP TABLE initiatives;
DROP TABLE initiative_ticks;
DROP TABLE initiative_schedules;
DROP TABLE initiative_sources;
