//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/wwsean08/action-items-bot/internal/audit"
)

func TestRecord_PersistsEntryWithBeforeAfterState(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	if _, err := repo.pool.Exec(ctx, "TRUNCATE audit_log"); err != nil {
		t.Fatalf("truncating audit_log: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	entry := audit.Entry{
		GuildID:   "guild1",
		UserID:    "user1",
		Username:  "alice",
		Action:    "config.set_channel",
		Reason:    audit.ReasonGuildOwner,
		Before:    map[string]string{"channel_id": "old-channel"},
		After:     map[string]string{"channel_id": "new-channel"},
		CreatedAt: now,
	}

	if err := repo.Record(ctx, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var (
		guildID, userID, username, action, reason string
		before, after                              *string
		createdAt                                  time.Time
	)
	row := repo.pool.QueryRow(ctx,
		`SELECT guild_id, user_id, username, action, reason, before_state::text, after_state::text, created_at FROM audit_log`)
	if err := row.Scan(&guildID, &userID, &username, &action, &reason, &before, &after, &createdAt); err != nil {
		t.Fatalf("scanning row: %v", err)
	}

	if guildID != "guild1" || userID != "user1" || username != "alice" {
		t.Errorf("guild_id/user_id/username = %q/%q/%q, want guild1/user1/alice", guildID, userID, username)
	}
	if action != "config.set_channel" || reason != "guild_owner" {
		t.Errorf("action/reason = %q/%q, want config.set_channel/guild_owner", action, reason)
	}
	if before == nil || *before != `{"channel_id": "old-channel"}` && *before != `{"channel_id":"old-channel"}` {
		t.Errorf("before_state = %v, want json containing old-channel", before)
	}
	if after == nil || *after != `{"channel_id": "new-channel"}` && *after != `{"channel_id":"new-channel"}` {
		t.Errorf("after_state = %v, want json containing new-channel", after)
	}
	if !createdAt.Equal(now) {
		t.Errorf("created_at = %v, want %v", createdAt, now)
	}
}

func TestRecord_PersistsNilBeforeAfterAsNull(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	if _, err := repo.pool.Exec(ctx, "TRUNCATE audit_log"); err != nil {
		t.Fatalf("truncating audit_log: %v", err)
	}

	entry := audit.Entry{
		GuildID:   "guild1",
		UserID:    "user1",
		Username:  "alice",
		Action:    "config.open",
		Reason:    audit.ReasonBotAdmin,
		CreatedAt: time.Now().UTC(),
	}

	if err := repo.Record(ctx, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var before, after *string
	row := repo.pool.QueryRow(ctx, `SELECT before_state::text, after_state::text FROM audit_log`)
	if err := row.Scan(&before, &after); err != nil {
		t.Fatalf("scanning row: %v", err)
	}
	if before != nil {
		t.Errorf("before_state = %v, want nil", *before)
	}
	if after != nil {
		t.Errorf("after_state = %v, want nil", *after)
	}
}
