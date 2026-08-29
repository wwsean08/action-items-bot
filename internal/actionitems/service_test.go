package actionitems

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestService() *Service {
	return NewService(newFakeRepository())
}

func TestCreateItem_SetsStatusNewAndGuild(t *testing.T) {
	s := newTestService()
	now := time.Now()

	item, err := s.CreateItem(context.Background(), "guild1", "buy milk", "user1", now)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if item.Status != StatusNew {
		t.Errorf("Status = %q, want %q", item.Status, StatusNew)
	}
	if item.GuildID != "guild1" {
		t.Errorf("GuildID = %q, want %q", item.GuildID, "guild1")
	}
	if item.ID == "" {
		t.Error("expected an ID to be assigned")
	}
}

func TestFindPendingByMessage_MatchesNewAndInProgressNotDone(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	newItem, _ := s.CreateItem(ctx, "guild1", "new item", "user1", now)
	_ = s.AttachMessage(ctx, newItem.ID, "msg-new")

	inProgressItem, _ := s.CreateItem(ctx, "guild1", "in progress item", "user1", now)
	_ = s.AttachMessage(ctx, inProgressItem.ID, "msg-in-progress")
	_ = s.MarkInProgress(ctx, inProgressItem.ID)

	doneItem, _ := s.CreateItem(ctx, "guild1", "done item", "user1", now)
	_ = s.AttachMessage(ctx, doneItem.ID, "msg-done")
	_ = s.CompleteItem(ctx, doneItem.ID, "approver1", now)

	if _, found, _ := s.FindPendingByMessage(ctx, "msg-new"); !found {
		t.Error("expected to find the new item")
	}
	if _, found, _ := s.FindPendingByMessage(ctx, "msg-in-progress"); !found {
		t.Error("expected to find the in-progress item")
	}
	if _, found, _ := s.FindPendingByMessage(ctx, "msg-done"); found {
		t.Error("expected not to find the done item")
	}
}

func TestMarkInProgress_FromNewSucceeds(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())

	if err := s.MarkInProgress(ctx, item.ID); err != nil {
		t.Fatalf("MarkInProgress() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
}

func TestMarkInProgress_FromInProgressFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	err := s.MarkInProgress(ctx, item.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestMarkNew_FromInProgressSucceeds(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	if err := s.MarkNew(ctx, item.ID); err != nil {
		t.Fatalf("MarkNew() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusNew {
		t.Errorf("Status = %q, want %q", got.Status, StatusNew)
	}
}

func TestMarkNew_FromNewFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())

	err := s.MarkNew(ctx, item.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestCompleteItem_FromNewRecordsPreviousStatusNew(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	now := time.Now()

	if err := s.CompleteItem(ctx, item.ID, "approver1", now); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, StatusDone)
	}
	if got.PreviousStatus != StatusNew {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, StatusNew)
	}
	if got.CompletedByUserID != "approver1" {
		t.Errorf("CompletedByUserID = %q, want %q", got.CompletedByUserID, "approver1")
	}
}

func TestCompleteItem_FromInProgressRecordsPreviousStatusInProgress(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	if err := s.CompleteItem(ctx, item.ID, "approver1", time.Now()); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.PreviousStatus != StatusInProgress {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, StatusInProgress)
	}
}

func TestCompleteItem_AlreadyDoneFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.CompleteItem(ctx, item.ID, "approver1", time.Now())

	err := s.CompleteItem(ctx, item.ID, "approver1", time.Now())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestListUndoable_ScopedToGuildAndWindow(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	inGuild, _ := s.CreateItem(ctx, "guild1", "recent", "user1", now)
	_ = s.CompleteItem(ctx, inGuild.ID, "approver1", now.Add(-1*time.Hour))

	otherGuild, _ := s.CreateItem(ctx, "guild2", "other guild", "user1", now)
	_ = s.CompleteItem(ctx, otherGuild.ID, "approver1", now.Add(-1*time.Hour))

	tooOld, _ := s.CreateItem(ctx, "guild1", "old", "user1", now)
	_ = s.CompleteItem(ctx, tooOld.ID, "approver1", now.Add(-30*time.Hour))

	items, err := s.ListUndoable(ctx, "guild1", now)
	if err != nil {
		t.Fatalf("ListUndoable() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ID != inGuild.ID {
		t.Errorf("items[0].ID = %q, want %q", items[0].ID, inGuild.ID)
	}
}

func TestUndoItem_RestoresRecordedPreviousStatus(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
	_ = s.MarkInProgress(ctx, item.ID)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	if err := s.UndoItem(ctx, item.ID, "new-message-id", now); err != nil {
		t.Fatalf("UndoItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
	if got.MessageID != "new-message-id" {
		t.Errorf("MessageID = %q, want %q", got.MessageID, "new-message-id")
	}
	if got.CompletedByUserID != "" {
		t.Errorf("CompletedByUserID = %q, want empty", got.CompletedByUserID)
	}
}

func TestUndoItem_OutsideWindowFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now.Add(-25*time.Hour))

	err := s.UndoItem(ctx, item.ID, "new-message-id", now)
	if !errors.Is(err, ErrNotUndoable) {
		t.Fatalf("err = %v, want ErrNotUndoable", err)
	}
}

func TestGuildConfig_DefaultsWhenUnconfigured(t *testing.T) {
	s := newTestService()
	cfg, err := s.GetGuildConfig(context.Background(), "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig() error = %v", err)
	}
	if cfg.InProgressEmote != DefaultInProgressEmote {
		t.Errorf("InProgressEmote = %q, want %q", cfg.InProgressEmote, DefaultInProgressEmote)
	}
	if cfg.DoneEmote != DefaultDoneEmote {
		t.Errorf("DoneEmote = %q, want %q", cfg.DoneEmote, DefaultDoneEmote)
	}
	if cfg.ActionItemsChannelID != "" {
		t.Errorf("ActionItemsChannelID = %q, want empty", cfg.ActionItemsChannelID)
	}
}

func TestSetActionItemsChannel_Persists(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	if err := s.SetActionItemsChannel(ctx, "guild1", "chan1"); err != nil {
		t.Fatalf("SetActionItemsChannel() error = %v", err)
	}

	cfg, _ := s.GetGuildConfig(ctx, "guild1")
	if cfg.ActionItemsChannelID != "chan1" {
		t.Errorf("ActionItemsChannelID = %q, want %q", cfg.ActionItemsChannelID, "chan1")
	}
}

func TestSetEmotes_RejectsEmptyValues(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	err := s.SetEmotes(ctx, "guild1", "", "✅")
	if !errors.Is(err, ErrInvalidEmote) {
		t.Fatalf("err = %v, want ErrInvalidEmote", err)
	}

	err = s.SetEmotes(ctx, "guild1", "🔄", "")
	if !errors.Is(err, ErrInvalidEmote) {
		t.Fatalf("err = %v, want ErrInvalidEmote", err)
	}
}

func TestSetEmotes_RejectsIdenticalEmotes(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	err := s.SetEmotes(ctx, "guild1", "✅", "✅")
	if !errors.Is(err, ErrInvalidEmote) {
		t.Fatalf("err = %v, want ErrInvalidEmote", err)
	}
}

func TestSetEmotes_ValidValuesPersist(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	if err := s.SetEmotes(ctx, "guild1", "👀", "🎉"); err != nil {
		t.Fatalf("SetEmotes() error = %v", err)
	}

	cfg, _ := s.GetGuildConfig(ctx, "guild1")
	if cfg.InProgressEmote != "👀" || cfg.DoneEmote != "🎉" {
		t.Errorf("emotes = %q/%q, want %q/%q", cfg.InProgressEmote, cfg.DoneEmote, "👀", "🎉")
	}
}

func TestApprovers_AddRemoveList(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	_ = s.AddApprover(ctx, "guild1", "user1")
	_ = s.AddApprover(ctx, "guild1", "user2")

	list, err := s.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	_ = s.RemoveApprover(ctx, "guild1", "user1")
	list, _ = s.ListApprovers(ctx, "guild1")
	if len(list) != 1 || list[0] != "user2" {
		t.Errorf("list = %v, want [user2]", list)
	}
}

func TestIsApprover_MatchesOwnerRoleOrUser(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	_ = s.AddApprover(ctx, "guild1", "user1")
	_ = s.SetApproverRole(ctx, "guild1", "role-a")

	ok, err := s.IsApprover(ctx, "guild1", "user1", nil)
	if err != nil || !ok {
		t.Errorf("IsApprover(user1) = %v, %v, want true, nil", ok, err)
	}

	ok, err = s.IsApprover(ctx, "guild1", "user2", []string{"role-a"})
	if err != nil || !ok {
		t.Errorf("IsApprover(user2 with role-a) = %v, %v, want true, nil", ok, err)
	}

	ok, err = s.IsApprover(ctx, "guild1", "user3", []string{"role-b"})
	if err != nil || ok {
		t.Errorf("IsApprover(user3) = %v, %v, want false, nil", ok, err)
	}
}

func TestSearchCompleted_MatchesSubstringCaseInsensitively(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "Buy Oat Milk", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "exact case", query: "Oat Milk", want: 1},
		{name: "lowercase query", query: "oat milk", want: 1},
		{name: "uppercase query", query: "OAT MILK", want: 1},
		{name: "interior substring", query: "at mi", want: 1},
		{name: "no match", query: "coffee", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := s.SearchCompleted(ctx, "guild1", tt.query)
			if err != nil {
				t.Fatalf("SearchCompleted() error = %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("len(got) = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestSearchCompleted_OnlyReturnsDoneItems(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	newItem, _ := s.CreateItem(ctx, "guild1", "task alpha", "user1", now)

	inProgress, _ := s.CreateItem(ctx, "guild1", "task beta", "user1", now)
	_ = s.MarkInProgress(ctx, inProgress.ID)

	done, _ := s.CreateItem(ctx, "guild1", "task gamma", "user1", now)
	_ = s.CompleteItem(ctx, done.ID, "approver1", now)

	got, _, err := s.SearchCompleted(ctx, "guild1", "task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != done.ID {
		t.Errorf("got[0].ID = %q, want %q (the done item)", got[0].ID, done.ID)
	}
	_ = newItem
}

func TestSearchCompleted_ScopedToGuild(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	mine, _ := s.CreateItem(ctx, "guild1", "shared description", "user1", now)
	_ = s.CompleteItem(ctx, mine.ID, "approver1", now)

	theirs, _ := s.CreateItem(ctx, "guild2", "shared description", "user1", now)
	_ = s.CompleteItem(ctx, theirs.ID, "approver1", now)

	got, _, err := s.SearchCompleted(ctx, "guild1", "shared")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != mine.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, mine.ID)
	}
}

func TestSearchCompleted_NoTimeWindow(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	ancient, _ := s.CreateItem(ctx, "guild1", "very old task", "user1", now)
	_ = s.CompleteItem(ctx, ancient.ID, "approver1", now.Add(-365*24*time.Hour))

	got, _, err := s.SearchCompleted(ctx, "guild1", "old task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (search has no time cutoff)", len(got))
	}
}

func TestSearchCompleted_CapsAtTenMostRecentFirst(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	for n := 0; n < 12; n++ {
		item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
		// n == 11 is the most recently completed.
		_ = s.CompleteItem(ctx, item.ID, "approver1", now.Add(time.Duration(n)*time.Minute))
	}

	got, hasMore, err := s.SearchCompleted(ctx, "guild1", "task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want 10", len(got))
	}
	if !hasMore {
		t.Error("hasMore = false, want true (12 items exist, only 10 shown)")
	}
	for idx := 1; idx < len(got); idx++ {
		if got[idx].CompletedAt.After(*got[idx-1].CompletedAt) {
			t.Fatalf("results not ordered most-recently-completed first at index %d", idx)
		}
	}
	if !got[0].CompletedAt.Equal(now.Add(11 * time.Minute)) {
		t.Errorf("got[0].CompletedAt = %v, want the newest completion %v", got[0].CompletedAt, now.Add(11*time.Minute))
	}
}

func TestSearchCompleted_HasMoreFalseWhenResultsFitWithinLimit(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "buy milk", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	got, hasMore, err := s.SearchCompleted(ctx, "guild1", "milk")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if hasMore {
		t.Error("hasMore = true, want false (only 1 item exists)")
	}
}

func TestSearchCompleted_RejectsBlankQuery(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	for _, query := range []string{"", "   ", "\t\n"} {
		_, _, err := s.SearchCompleted(ctx, "guild1", query)
		if !errors.Is(err, ErrEmptyQuery) {
			t.Errorf("SearchCompleted(%q) err = %v, want ErrEmptyQuery", query, err)
		}
	}
}

func TestSearchCompleted_TrimsSurroundingWhitespace(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "buy milk", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	got, _, err := s.SearchCompleted(ctx, "guild1", "  milk  ")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}

func TestSearchCompleted_NoMatchesReturnsEmptyNotError(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	got, _, err := s.SearchCompleted(ctx, "guild1", "nothing here")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
