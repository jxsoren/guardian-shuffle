CREATE TABLE IF NOT EXISTS activity_states (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    char_id       TEXT        NOT NULL DEFAULT '',
    activity_hash BIGINT      NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
