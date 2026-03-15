CREATE TABLE IF NOT EXISTS doctor_reports (
    id          BIGSERIAL PRIMARY KEY,
    status      TEXT        NOT NULL DEFAULT 'healthy',
    checked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    report_json JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_doctor_reports_checked_at ON doctor_reports (checked_at DESC);
