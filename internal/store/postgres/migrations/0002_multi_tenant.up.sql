-- This migration adds `guild_id TEXT NOT NULL` to action_items with no
-- default. It will fail with "column contains null values" if action_items
-- has any existing rows. If upgrading a pre-multi-tenant deployment,
-- truncate the table first: TRUNCATE action_items;
-- If this migration fails partway through, golang-migrate will leave the
-- schema_migrations table marked dirty; after fixing the underlying issue,
-- you'll need `migrate force <version>` (or your driver's equivalent) to
-- clear the dirty flag before retrying.
ALTER TABLE action_items ADD COLUMN guild_id TEXT NOT NULL;
ALTER TABLE action_items ADD COLUMN previous_status TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_action_items_message_id;
CREATE INDEX idx_action_items_message_id ON action_items (message_id) WHERE status <> 'done';

DROP INDEX IF EXISTS idx_action_items_completed_at;
CREATE INDEX idx_action_items_completed_at ON action_items (completed_at) WHERE status = 'done';

CREATE INDEX idx_action_items_guild_id ON action_items (guild_id);

CREATE TABLE guild_configs (
    guild_id TEXT PRIMARY KEY,
    action_items_channel_id TEXT NOT NULL DEFAULT '',
    approver_role_id TEXT NOT NULL DEFAULT '',
    in_progress_emote TEXT NOT NULL DEFAULT '🔄',
    done_emote TEXT NOT NULL DEFAULT '✅',
    help_message_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE approvers (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (guild_id, user_id)
);
