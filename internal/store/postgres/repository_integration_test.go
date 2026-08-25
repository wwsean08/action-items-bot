//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://action_items:action_items@localhost:5432/action_items?sslmode=disable"
	}
	return url
}

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	url := testDatabaseURL(t)

	if err := Migrate(url); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	repo, err := New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(repo.Close)

	if _, err := repo.pool.Exec(context.Background(), "TRUNCATE action_items, guild_configs, approvers"); err != nil {
		t.Fatalf("truncating tables: %v", err)
	}

	return repo
}

func TestRepository_Ping_SucceedsAgainstLiveDatabase(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	if err := repo.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestRepository_Ping_FailsAfterClose(t *testing.T) {
	ctx := context.Background()
	url := testDatabaseURL(t)
	if err := Migrate(url); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	repo, err := New(ctx, url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	repo.Close()

	if err := repo.Ping(ctx); err == nil {
		t.Fatal("expected error when pinging a closed pool")
	}
}

func TestRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID:         "guild1",
		Description:     "buy milk",
		CreatedByUserID: "user1",
		CreatedAt:       now,
		Status:          actionitems.StatusNew,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected an ID to be assigned")
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "buy milk" {
		t.Errorf("Description = %q, want %q", got.Description, "buy milk")
	}
	if got.CreatedByUserID != "user1" {
		t.Errorf("CreatedByUserID = %q, want %q", got.CreatedByUserID, "user1")
	}
	if got.Status != actionitems.StatusNew {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusNew)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
}

func TestRepository_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	_, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000000")

	if err != actionitems.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_UpdateMessageID(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})

	if err := repo.UpdateMessageID(ctx, item.ID, "msg123"); err != nil {
		t.Fatalf("UpdateMessageID: %v", err)
	}

	got, _ := repo.Get(ctx, item.ID)
	if got.MessageID != "msg123" {
		t.Errorf("MessageID = %q, want %q", got.MessageID, "msg123")
	}
}

func TestRepository_FindPendingByMessageID(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	_ = repo.UpdateMessageID(ctx, item.ID, "msg123")

	found, err := repo.FindPendingByMessageID(ctx, "msg123")
	if err != nil {
		t.Fatalf("FindPendingByMessageID: %v", err)
	}
	if found.ID != item.ID {
		t.Errorf("ID = %q, want %q", found.ID, item.ID)
	}

	_, err = repo.FindPendingByMessageID(ctx, "no-such-message")
	if err != actionitems.ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_CompleteAndReopen(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	completedAt := time.Now().UTC().Truncate(time.Microsecond)

	if err := repo.Complete(ctx, item.ID, "approver1", completedAt, actionitems.StatusNew); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusDone)
	}
	if got.CompletedByUserID != "approver1" {
		t.Errorf("CompletedByUserID = %q, want %q", got.CompletedByUserID, "approver1")
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}

	if err := repo.Reopen(ctx, item.ID, "newmsg456", actionitems.StatusNew); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusNew {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusNew)
	}
	if got.MessageID != "newmsg456" {
		t.Errorf("MessageID = %q, want %q", got.MessageID, "newmsg456")
	}
	if got.CompletedByUserID != "" {
		t.Errorf("CompletedByUserID = %q, want empty", got.CompletedByUserID)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", got.CompletedAt)
	}
	if got.PreviousStatus != "" {
		t.Errorf("PreviousStatus = %q, want empty", got.PreviousStatus)
	}
}

func TestRepository_ListCompletedSince(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	recent, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "recent", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, recent.ID, "approver1", now.Add(-1*time.Hour), actionitems.StatusNew)

	old, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "old", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, old.ID, "approver1", now.Add(-30*time.Hour), actionitems.StatusNew)

	got, err := repo.ListCompletedSince(ctx, "guild1", now.Add(-24*time.Hour), 5)
	if err != nil {
		t.Fatalf("ListCompletedSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != recent.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, recent.ID)
	}
}

func TestRepository_GuildConfig_DefaultsWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	cfg, err := repo.GetGuildConfig(ctx, "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig: %v", err)
	}
	if cfg.InProgressEmote != actionitems.DefaultInProgressEmote {
		t.Errorf("InProgressEmote = %q, want %q", cfg.InProgressEmote, actionitems.DefaultInProgressEmote)
	}
	if cfg.DoneEmote != actionitems.DefaultDoneEmote {
		t.Errorf("DoneEmote = %q, want %q", cfg.DoneEmote, actionitems.DefaultDoneEmote)
	}
	if cfg.ActionItemsChannelID != "" {
		t.Errorf("ActionItemsChannelID = %q, want empty", cfg.ActionItemsChannelID)
	}
}

func TestRepository_GuildConfig_SetAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	if err := repo.SetActionItemsChannel(ctx, "guild1", "chan1"); err != nil {
		t.Fatalf("SetActionItemsChannel: %v", err)
	}
	if err := repo.SetApproverRole(ctx, "guild1", "role1"); err != nil {
		t.Fatalf("SetApproverRole: %v", err)
	}
	if err := repo.SetEmotes(ctx, "guild1", "👀", "🎉"); err != nil {
		t.Fatalf("SetEmotes: %v", err)
	}
	if err := repo.SetHelpMessageID(ctx, "guild1", "help-msg-1"); err != nil {
		t.Fatalf("SetHelpMessageID: %v", err)
	}

	cfg, err := repo.GetGuildConfig(ctx, "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig: %v", err)
	}
	if cfg.ActionItemsChannelID != "chan1" {
		t.Errorf("ActionItemsChannelID = %q, want %q", cfg.ActionItemsChannelID, "chan1")
	}
	if cfg.ApproverRoleID != "role1" {
		t.Errorf("ApproverRoleID = %q, want %q", cfg.ApproverRoleID, "role1")
	}
	if cfg.InProgressEmote != "👀" || cfg.DoneEmote != "🎉" {
		t.Errorf("emotes = %q/%q, want 👀/🎉", cfg.InProgressEmote, cfg.DoneEmote)
	}
	if cfg.HelpMessageID != "help-msg-1" {
		t.Errorf("HelpMessageID = %q, want %q", cfg.HelpMessageID, "help-msg-1")
	}
}

func TestRepository_Approvers_AddRemoveList(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	if err := repo.AddApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("AddApprover: %v", err)
	}
	if err := repo.AddApprover(ctx, "guild1", "user2"); err != nil {
		t.Fatalf("AddApprover: %v", err)
	}
	// idempotent re-add
	if err := repo.AddApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("AddApprover (repeat): %v", err)
	}

	list, err := repo.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	if err := repo.RemoveApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("RemoveApprover: %v", err)
	}
	list, err = repo.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers: %v", err)
	}
	if len(list) != 1 || list[0] != "user2" {
		t.Errorf("list = %v, want [user2]", list)
	}
}

func TestRepository_StateTransitions_SetStatusAndComplete(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "task", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.SetStatus(ctx, item.ID, actionitems.StatusInProgress); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusInProgress)
	}

	completedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Complete(ctx, item.ID, "approver1", completedAt, actionitems.StatusInProgress); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusDone)
	}
	if got.PreviousStatus != actionitems.StatusInProgress {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, actionitems.StatusInProgress)
	}

	if err := repo.Reopen(ctx, item.ID, "new-msg", actionitems.StatusInProgress); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusInProgress)
	}
	if got.PreviousStatus != "" {
		t.Errorf("PreviousStatus = %q, want empty", got.PreviousStatus)
	}
}

func TestRepository_FindPendingByMessageID_ExcludesDoneOnly(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "task", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	_ = repo.UpdateMessageID(ctx, item.ID, "msg1")
	_ = repo.SetStatus(ctx, item.ID, actionitems.StatusInProgress)

	found, err := repo.FindPendingByMessageID(ctx, "msg1")
	if err != nil {
		t.Fatalf("FindPendingByMessageID: %v", err)
	}
	if found.ID != item.ID {
		t.Errorf("ID = %q, want %q", found.ID, item.ID)
	}

	_ = repo.Complete(ctx, item.ID, "approver1", time.Now(), actionitems.StatusInProgress)
	_, err = repo.FindPendingByMessageID(ctx, "msg1")
	if err != actionitems.ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_ListCompletedSince_ScopedToGuild(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	inGuild, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "in guild", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, inGuild.ID, "approver1", now.Add(-1*time.Hour), actionitems.StatusNew)

	otherGuild, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild2", Description: "other guild", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, otherGuild.ID, "approver1", now.Add(-1*time.Hour), actionitems.StatusNew)

	got, err := repo.ListCompletedSince(ctx, "guild1", now.Add(-24*time.Hour), 5)
	if err != nil {
		t.Fatalf("ListCompletedSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != inGuild.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, inGuild.ID)
	}
}
