ALTER TABLE approvals
    ADD COLUMN IF NOT EXISTS approval_type TEXT NOT NULL DEFAULT 'tool_call',
    ADD COLUMN IF NOT EXISTS payload_json JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'approvals_approval_type_check'
    ) THEN
        ALTER TABLE approvals
            ADD CONSTRAINT approvals_approval_type_check
            CHECK (approval_type IN ('tool_call', 'browser_tab_selection'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_approvals_approval_type_requested_at
    ON approvals (approval_type, requested_at DESC);
