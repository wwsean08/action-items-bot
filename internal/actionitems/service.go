package actionitems

import (
	"context"
	"errors"
	"time"
)

const (
	undoWindow = 24 * time.Hour
	undoLimit  = 5
)

var (
	ErrNotUndoable       = errors.New("action item is not eligible for undo")
	ErrInvalidTransition = errors.New("action item is not in a state that allows this transition")
	ErrInvalidEmote      = errors.New("emote must not be empty")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateItem(ctx context.Context, guildID, description, createdByUserID string, now time.Time) (ActionItem, error) {
	return s.repo.Create(ctx, ActionItem{
		GuildID:         guildID,
		Description:     description,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		Status:          StatusNew,
	})
}

func (s *Service) AttachMessage(ctx context.Context, id, messageID string) error {
	return s.repo.UpdateMessageID(ctx, id, messageID)
}

func (s *Service) FindPendingByMessage(ctx context.Context, messageID string) (ActionItem, bool, error) {
	item, err := s.repo.FindPendingByMessageID(ctx, messageID)
	if errors.Is(err, ErrNotFound) {
		return ActionItem{}, false, nil
	}
	if err != nil {
		return ActionItem{}, false, err
	}
	return item, true, nil
}

func (s *Service) GetItem(ctx context.Context, id string) (ActionItem, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) MarkInProgress(ctx context.Context, id string) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusNew {
		return ErrInvalidTransition
	}
	return s.repo.SetStatus(ctx, id, StatusInProgress)
}

func (s *Service) MarkNew(ctx context.Context, id string) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusInProgress {
		return ErrInvalidTransition
	}
	return s.repo.SetStatus(ctx, id, StatusNew)
}

func (s *Service) CompleteItem(ctx context.Context, id, completedByUserID string, now time.Time) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status == StatusDone {
		return ErrInvalidTransition
	}
	return s.repo.Complete(ctx, id, completedByUserID, now, item.Status)
}

func (s *Service) ListUndoable(ctx context.Context, guildID string, now time.Time) ([]ActionItem, error) {
	return s.repo.ListCompletedSince(ctx, guildID, now.Add(-undoWindow), undoLimit)
}

func (s *Service) UndoItem(ctx context.Context, id, newMessageID string, now time.Time) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusDone || item.CompletedAt == nil || item.CompletedAt.Before(now.Add(-undoWindow)) {
		return ErrNotUndoable
	}
	restoreStatus := item.PreviousStatus
	if restoreStatus == "" {
		restoreStatus = StatusNew
	}
	return s.repo.Reopen(ctx, id, newMessageID, restoreStatus)
}

func (s *Service) GetGuildConfig(ctx context.Context, guildID string) (GuildConfig, error) {
	return s.repo.GetGuildConfig(ctx, guildID)
}

func (s *Service) SetActionItemsChannel(ctx context.Context, guildID, channelID string) error {
	return s.repo.SetActionItemsChannel(ctx, guildID, channelID)
}

func (s *Service) SetApproverRole(ctx context.Context, guildID, roleID string) error {
	return s.repo.SetApproverRole(ctx, guildID, roleID)
}

func (s *Service) SetEmotes(ctx context.Context, guildID, inProgressEmote, doneEmote string) error {
	if inProgressEmote == "" || doneEmote == "" {
		return ErrInvalidEmote
	}
	if inProgressEmote == doneEmote {
		return ErrInvalidEmote
	}
	return s.repo.SetEmotes(ctx, guildID, inProgressEmote, doneEmote)
}

func (s *Service) SetHelpMessageID(ctx context.Context, guildID, messageID string) error {
	return s.repo.SetHelpMessageID(ctx, guildID, messageID)
}

func (s *Service) AddApprover(ctx context.Context, guildID, userID string) error {
	return s.repo.AddApprover(ctx, guildID, userID)
}

func (s *Service) RemoveApprover(ctx context.Context, guildID, userID string) error {
	return s.repo.RemoveApprover(ctx, guildID, userID)
}

func (s *Service) ListApprovers(ctx context.Context, guildID string) ([]string, error) {
	return s.repo.ListApprovers(ctx, guildID)
}

func (s *Service) IsApprover(ctx context.Context, guildID, userID string, memberRoleIDs []string) (bool, error) {
	cfg, err := s.repo.GetGuildConfig(ctx, guildID)
	if err != nil {
		return false, err
	}
	approvers, err := s.repo.ListApprovers(ctx, guildID)
	if err != nil {
		return false, err
	}
	return isApproverMatch(userID, memberRoleIDs, approvers, cfg.ApproverRoleID), nil
}
