package actionitems

import (
	"context"
	"testing"
	"time"
)

func TestCreateItem_CreatesPendingItem(t *testing.T) {
	svc := NewService(newFakeRepository())
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	item, err := svc.CreateItem(context.Background(), "buy milk", "user1", now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if item.Description != "buy milk" {
		t.Errorf("Description = %q, want %q", item.Description, "buy milk")
	}
	if item.CreatedByUserID != "user1" {
		t.Errorf("CreatedByUserID = %q, want %q", item.CreatedByUserID, "user1")
	}
	if item.Status != StatusPending {
		t.Errorf("Status = %q, want %q", item.Status, StatusPending)
	}
	if !item.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", item.CreatedAt, now)
	}
}

func TestAttachMessage_SetsMessageID(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", time.Now())

	err := svc.AttachMessage(ctx, item.ID, "msg123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.MessageID != "msg123" {
		t.Errorf("MessageID = %q, want %q", got.MessageID, "msg123")
	}
}

func TestFindPendingByMessage_ReturnsFalseWhenNotFound(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newFakeRepository())

	_, found, err := svc.FindPendingByMessage(ctx, "no-such-message")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found = false")
	}
}

func TestFindPendingByMessage_ReturnsItemWhenFound(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", time.Now())
	_ = svc.AttachMessage(ctx, item.ID, "msg123")

	found, ok, err := svc.FindPendingByMessage(ctx, "msg123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected found = true")
	}
	if found.ID != item.ID {
		t.Errorf("ID = %q, want %q", found.ID, item.ID)
	}
}

func TestCompleteItem_MarksCompleted(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", time.Now())
	completedAt := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)

	err := svc.CompleteItem(ctx, item.ID, "approver1", completedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, StatusCompleted)
	}
	if got.CompletedByUserID != "approver1" {
		t.Errorf("CompletedByUserID = %q, want %q", got.CompletedByUserID, "approver1")
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}
}

func TestGetItem_ReturnsItemByID(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newFakeRepository())
	created, _ := svc.CreateItem(ctx, "buy milk", "user1", time.Now())

	got, err := svc.GetItem(ctx, created.ID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGetItem_ReturnsErrNotFoundForUnknownID(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newFakeRepository())

	_, err := svc.GetItem(ctx, "no-such-id")

	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListUndoable_ExcludesItemsOlderThan24Hours(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	recent, _ := svc.CreateItem(ctx, "recent", "user1", now.Add(-2*time.Hour))
	_ = svc.CompleteItem(ctx, recent.ID, "approver1", now.Add(-1*time.Hour))

	old, _ := svc.CreateItem(ctx, "old", "user1", now.Add(-30*time.Hour))
	_ = svc.CompleteItem(ctx, old.ID, "approver1", now.Add(-25*time.Hour))

	got, err := svc.ListUndoable(ctx, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != recent.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, recent.ID)
	}
}

func TestListUndoable_LimitsToFiveNewestFirst(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	var ids []string
	for i := 0; i < 6; i++ {
		item, _ := svc.CreateItem(ctx, "item", "user1", now)
		completedAt := now.Add(-time.Duration(i) * time.Minute)
		_ = svc.CompleteItem(ctx, item.ID, "approver1", completedAt)
		ids = append(ids, item.ID)
	}

	got, err := svc.ListUndoable(ctx, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len(got) = %d, want 5", len(got))
	}
	// Newest completed (i=0, ids[0]) should be first.
	if got[0].ID != ids[0] {
		t.Errorf("got[0].ID = %q, want %q (newest first)", got[0].ID, ids[0])
	}
}

func TestUndoItem_ReopensCompletedItemWithinWindow(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", now)
	_ = svc.CompleteItem(ctx, item.ID, "approver1", now.Add(-1*time.Hour))

	err := svc.UndoItem(ctx, item.ID, "newmsg456", now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, StatusPending)
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

func TestUndoItem_FailsWhenItemIsStillPending(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", time.Now())

	err := svc.UndoItem(ctx, item.ID, "newmsg456", time.Now())

	if err == nil {
		t.Fatal("expected error when undoing a pending item")
	}
}

func TestUndoItem_FailsWhenOutsideWindow(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	item, _ := svc.CreateItem(ctx, "buy milk", "user1", now)
	_ = svc.CompleteItem(ctx, item.ID, "approver1", now.Add(-25*time.Hour))

	err := svc.UndoItem(ctx, item.ID, "newmsg456", now)

	if err == nil {
		t.Fatal("expected error when undoing an item outside the 24h window")
	}
}
