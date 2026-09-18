CREATE TABLE audit_log (
    id           BIGSERIAL PRIMARY KEY,
    guild_id     TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    username     TEXT NOT NULL,
    action       TEXT NOT NULL,
    reason       TEXT NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    created_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_audit_log_guild_id ON audit_log (guild_id);
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at);
