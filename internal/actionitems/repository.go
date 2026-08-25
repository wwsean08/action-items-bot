package actionitems

import (
	"context"
	"time"
)

// Repository persists ActionItems. Implementations must return ErrNotFound
// when a lookup finds no matching row.
type Repository interface {
	Create(ctx context.Context, item ActionItem) (ActionItem, error)
	Get(ctx context.Context, id string) (ActionItem, error)
	UpdateMessageID(ctx context.Context, id, messageID string) error
	FindPendingByMessageID(ctx context.Context, messageID string) (ActionItem, error)
	Complete(ctx context.Context, id, completedByUserID string, completedAt time.Time) error
	ListCompletedSince(ctx context.Context, since time.Time, limit int) ([]ActionItem, error)
	Reopen(ctx context.Context, id, newMessageID string) error
}
