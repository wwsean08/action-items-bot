package actionitems

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// fakeRepository is an in-memory Repository used only in tests.
type fakeRepository struct {
	items  map[string]ActionItem
	nextID int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{items: make(map[string]ActionItem)}
}

func (f *fakeRepository) Create(_ context.Context, item ActionItem) (ActionItem, error) {
	f.nextID++
	item.ID = strconv.Itoa(f.nextID)
	f.items[item.ID] = item
	return item, nil
}

func (f *fakeRepository) Get(_ context.Context, id string) (ActionItem, error) {
	item, ok := f.items[id]
	if !ok {
		return ActionItem{}, ErrNotFound
	}
	return item, nil
}

func (f *fakeRepository) UpdateMessageID(_ context.Context, id, messageID string) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.MessageID = messageID
	f.items[id] = item
	return nil
}

func (f *fakeRepository) FindPendingByMessageID(_ context.Context, messageID string) (ActionItem, error) {
	for _, item := range f.items {
		if item.MessageID == messageID && item.Status == StatusPending {
			return item, nil
		}
	}
	return ActionItem{}, ErrNotFound
}

func (f *fakeRepository) Complete(_ context.Context, id, completedByUserID string, completedAt time.Time) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = StatusCompleted
	item.CompletedByUserID = completedByUserID
	ca := completedAt
	item.CompletedAt = &ca
	f.items[id] = item
	return nil
}

func (f *fakeRepository) ListCompletedSince(_ context.Context, since time.Time, limit int) ([]ActionItem, error) {
	var result []ActionItem
	for _, item := range f.items {
		if item.Status == StatusCompleted && item.CompletedAt != nil && !item.CompletedAt.Before(since) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CompletedAt.After(*result[j].CompletedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepository) Reopen(_ context.Context, id, newMessageID string) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = StatusPending
	item.MessageID = newMessageID
	item.CompletedByUserID = ""
	item.CompletedAt = nil
	f.items[id] = item
	return nil
}
