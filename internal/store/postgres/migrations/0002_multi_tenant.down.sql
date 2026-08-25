DROP TABLE IF EXISTS approvers;
DROP TABLE IF EXISTS guild_configs;

DROP INDEX IF EXISTS idx_action_items_guild_id;

DROP INDEX IF EXISTS idx_action_items_completed_at;
CREATE INDEX idx_action_items_completed_at ON action_items (completed_at) WHERE status = 'completed';

DROP INDEX IF EXISTS idx_action_items_message_id;
CREATE INDEX idx_action_items_message_id ON action_items (message_id) WHERE status = 'pending';

ALTER TABLE action_items DROP COLUMN previous_status;
ALTER TABLE action_items DROP COLUMN guild_id;
