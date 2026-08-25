package actionitems

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// fakeRepository is an in-memory Repository used only in tests.
type fakeRepository struct {
	items     map[string]ActionItem
	nextID    int
	configs   map[string]GuildConfig
	approvers map[string]map[string]bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		items:     make(map[string]ActionItem),
		configs:   make(map[string]GuildConfig),
		approvers: make(map[string]map[string]bool),
	}
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
		if item.MessageID == messageID && item.Status != StatusDone {
			return item, nil
		}
	}
	return ActionItem{}, ErrNotFound
}

func (f *fakeRepository) SetStatus(_ context.Context, id string, status Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = status
	f.items[id] = item
	return nil
}

func (f *fakeRepository) Complete(_ context.Context, id, completedByUserID string, completedAt time.Time, previousStatus Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = StatusDone
	item.PreviousStatus = previousStatus
	item.CompletedByUserID = completedByUserID
	ca := completedAt
	item.CompletedAt = &ca
	f.items[id] = item
	return nil
}

func (f *fakeRepository) ListCompletedSince(_ context.Context, guildID string, since time.Time, limit int) ([]ActionItem, error) {
	var result []ActionItem
	for _, item := range f.items {
		if item.GuildID == guildID && item.Status == StatusDone && item.CompletedAt != nil && !item.CompletedAt.Before(since) {
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

func (f *fakeRepository) Reopen(_ context.Context, id, newMessageID string, restoreStatus Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = restoreStatus
	item.MessageID = newMessageID
	item.CompletedByUserID = ""
	item.CompletedAt = nil
	item.PreviousStatus = ""
	f.items[id] = item
	return nil
}

func (f *fakeRepository) getOrCreateConfig(guildID string) GuildConfig {
	cfg, ok := f.configs[guildID]
	if !ok {
		cfg = GuildConfig{GuildID: guildID, InProgressEmote: DefaultInProgressEmote, DoneEmote: DefaultDoneEmote}
	}
	return cfg
}

func (f *fakeRepository) GetGuildConfig(_ context.Context, guildID string) (GuildConfig, error) {
	return f.getOrCreateConfig(guildID), nil
}

func (f *fakeRepository) SetActionItemsChannel(_ context.Context, guildID, channelID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.ActionItemsChannelID = channelID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetApproverRole(_ context.Context, guildID, roleID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.ApproverRoleID = roleID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetEmotes(_ context.Context, guildID, inProgressEmote, doneEmote string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.InProgressEmote = inProgressEmote
	cfg.DoneEmote = doneEmote
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetHelpMessageID(_ context.Context, guildID, messageID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.HelpMessageID = messageID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) AddApprover(_ context.Context, guildID, userID string) error {
	if f.approvers[guildID] == nil {
		f.approvers[guildID] = make(map[string]bool)
	}
	f.approvers[guildID][userID] = true
	return nil
}

func (f *fakeRepository) RemoveApprover(_ context.Context, guildID, userID string) error {
	delete(f.approvers[guildID], userID)
	return nil
}

func (f *fakeRepository) ListApprovers(_ context.Context, guildID string) ([]string, error) {
	var result []string
	for userID := range f.approvers[guildID] {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result, nil
}
