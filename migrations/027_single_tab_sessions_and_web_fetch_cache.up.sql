CREATE TABLE IF NOT EXISTS single_tab_sessions (
    single_tab_session_id TEXT PRIMARY KEY,
    session_key           TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    created_by_run_id     TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    approval_id           TEXT REFERENCES approvals (approval_id) ON DELETE SET NULL,
    status                TEXT NOT NULL,
    bound_tab_ref         TEXT NOT NULL,
    browser_instance_id   TEXT NOT NULL DEFAULT '',
    host_id               TEXT NOT NULL DEFAULT '',
    selected_via          TEXT NOT NULL DEFAULT '',
    selected_by           TEXT NOT NULL DEFAULT '',
    current_url           TEXT NOT NULL DEFAULT '',
    current_title         TEXT NOT NULL DEFAULT '',
    status_reason         TEXT NOT NULL DEFAULT '',
    metadata_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at          TIMESTAMPTZ,
    last_seen_at          TIMESTAMPTZ,
    released_at           TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT single_tab_sessions_status_check CHECK (status IN ('PENDING_APPROVAL', 'ACTIVE', 'TAB_CLOSED', 'REVOKED_BY_USER', 'EXPIRED', 'HOST_DISCONNECTED')),
    CONSTRAINT single_tab_sessions_selected_via_check CHECK (selected_via IN ('', 'web', 'telegram', 'system'))
);

CREATE INDEX IF NOT EXISTS idx_single_tab_sessions_session_key
    ON single_tab_sessions (session_key, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_single_tab_sessions_status
    ON single_tab_sessions (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_single_tab_sessions_approval_id
    ON single_tab_sessions (approval_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_single_tab_sessions_active_per_session
    ON single_tab_sessions (session_key)
    WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS approval_tab_candidates (
    approval_tab_candidate_id BIGSERIAL PRIMARY KEY,
    approval_id               TEXT NOT NULL REFERENCES approvals (approval_id) ON DELETE CASCADE,
    candidate_token           TEXT NOT NULL,
    internal_tab_ref          TEXT NOT NULL,
    title                     TEXT NOT NULL DEFAULT '',
    domain                    TEXT NOT NULL DEFAULT '',
    current_url               TEXT NOT NULL DEFAULT '',
    favicon_url               TEXT NOT NULL DEFAULT '',
    display_label             TEXT NOT NULL DEFAULT '',
    status                    TEXT NOT NULL DEFAULT 'available',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    selected_at               TIMESTAMPTZ,
    expires_at                TIMESTAMPTZ,
    metadata_json             JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT approval_tab_candidates_status_check CHECK (status IN ('available', 'selected', 'expired', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_tab_candidates_token
    ON approval_tab_candidates (candidate_token);

CREATE INDEX IF NOT EXISTS idx_approval_tab_candidates_approval_id
    ON approval_tab_candidates (approval_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_tab_candidates_selected_per_approval
    ON approval_tab_candidates (approval_id)
    WHERE status = 'selected';

CREATE TABLE IF NOT EXISTS web_fetch_cache (
    cache_key         TEXT PRIMARY KEY,
    requested_url     TEXT NOT NULL,
    final_url         TEXT NOT NULL DEFAULT '',
    provider          TEXT NOT NULL,
    status_code       INTEGER NOT NULL DEFAULT 0,
    content_type      TEXT NOT NULL DEFAULT '',
    text_content      TEXT NOT NULL DEFAULT '',
    html_content      TEXT NOT NULL DEFAULT '',
    metadata_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    fetched_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web_fetch_cache_provider_check CHECK (provider IN ('self_hosted_primary', 'jina_reader_fallback', 'plain_http_fallback'))
);

CREATE INDEX IF NOT EXISTS idx_web_fetch_cache_requested_url
    ON web_fetch_cache (requested_url);

CREATE INDEX IF NOT EXISTS idx_web_fetch_cache_expires_at
    ON web_fetch_cache (expires_at);

CREATE INDEX IF NOT EXISTS idx_web_fetch_cache_fetched_at
    ON web_fetch_cache (fetched_at DESC);
