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
