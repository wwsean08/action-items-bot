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

	if _, err := repo.pool.Exec(context.Background(), "TRUNCATE action_items"); err != nil {
		t.Fatalf("truncating table: %v", err)
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
		Description:     "buy milk",
		CreatedByUserID: "user1",
		CreatedAt:       now,
		Status:          actionitems.StatusPending,
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
	if got.Status != actionitems.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusPending)
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
		Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusPending,
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
		Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusPending,
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
		Description: "buy milk", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusPending,
	})
	completedAt := time.Now().UTC().Truncate(time.Microsecond)

	if err := repo.Complete(ctx, item.ID, "approver1", completedAt); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusCompleted)
	}
	if got.CompletedByUserID != "approver1" {
		t.Errorf("CompletedByUserID = %q, want %q", got.CompletedByUserID, "approver1")
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}

	if err := repo.Reopen(ctx, item.ID, "newmsg456"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusPending)
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
}

func TestRepository_ListCompletedSince(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	recent, _ := repo.Create(ctx, actionitems.ActionItem{
		Description: "recent", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusPending,
	})
	_ = repo.Complete(ctx, recent.ID, "approver1", now.Add(-1*time.Hour))

	old, _ := repo.Create(ctx, actionitems.ActionItem{
		Description: "old", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusPending,
	})
	_ = repo.Complete(ctx, old.ID, "approver1", now.Add(-30*time.Hour))

	got, err := repo.ListCompletedSince(ctx, now.Add(-24*time.Hour), 5)
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
