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

var ErrNotUndoable = errors.New("action item is not eligible for undo")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateItem(ctx context.Context, description, createdByUserID string, now time.Time) (ActionItem, error) {
	return s.repo.Create(ctx, ActionItem{
		Description:     description,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		Status:          StatusPending,
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

func (s *Service) CompleteItem(ctx context.Context, id, completedByUserID string, now time.Time) error {
	return s.repo.Complete(ctx, id, completedByUserID, now)
}

func (s *Service) ListUndoable(ctx context.Context, now time.Time) ([]ActionItem, error) {
	return s.repo.ListCompletedSince(ctx, now.Add(-undoWindow), undoLimit)
}

func (s *Service) UndoItem(ctx context.Context, id, newMessageID string, now time.Time) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusCompleted || item.CompletedAt == nil || item.CompletedAt.Before(now.Add(-undoWindow)) {
		return ErrNotUndoable
	}
	return s.repo.Reopen(ctx, id, newMessageID)
}
