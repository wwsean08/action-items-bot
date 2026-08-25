CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE action_items (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description          TEXT NOT NULL,
    created_by_user_id   TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL,
    message_id           TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL,
    completed_by_user_id TEXT NOT NULL DEFAULT '',
    completed_at         TIMESTAMPTZ
);

CREATE INDEX idx_action_items_message_id ON action_items (message_id) WHERE status = 'pending';
CREATE INDEX idx_action_items_completed_at ON action_items (completed_at) WHERE status = 'completed';
