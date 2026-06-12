CREATE TABLE IF NOT EXISTS users (
    id                    BIGSERIAL PRIMARY KEY,
    bungie_membership_id  TEXT NOT NULL UNIQUE,
    membership_type       BIGINT NOT NULL,
    primary_character_id  TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tokens (
    user_id            BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token_enc   BYTEA NOT NULL,
    refresh_token_enc  BYTEA NOT NULL,
    access_expires_at  TIMESTAMPTZ NOT NULL,
    refresh_expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    user_id          BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled          BOOLEAN NOT NULL DEFAULT false,
    trigger_mode     TEXT NOT NULL DEFAULT 'manual',
    interval_seconds BIGINT NOT NULL DEFAULT 0,
    last_cycled_at   TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS swap_history (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_hash  BIGINT NOT NULL,
    to_hash    BIGINT NOT NULL,
    status     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
