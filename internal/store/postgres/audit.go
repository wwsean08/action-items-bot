package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wwsean08/action-items-bot/internal/audit"
)

var _ audit.Repository = (*Repository)(nil)

func (r *Repository) Record(ctx context.Context, entry audit.Entry) error {
	before, err := marshalAuditState(entry.Before)
	if err != nil {
		return fmt.Errorf("marshaling before state: %w", err)
	}
	after, err := marshalAuditState(entry.After)
	if err != nil {
		return fmt.Errorf("marshaling after state: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_log (guild_id, user_id, username, action, reason, before_state, after_state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)`,
		entry.GuildID, entry.UserID, entry.Username, entry.Action, string(entry.Reason), before, after, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting audit log entry: %w", err)
	}
	return nil
}

// marshalAuditState returns nil (which binds as SQL NULL) for a nil map,
// otherwise its JSON encoding as a string for the ::jsonb cast to parse.
func marshalAuditState(state map[string]string) (*string, error) {
	if state == nil {
		return nil, nil
	}
	b, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
