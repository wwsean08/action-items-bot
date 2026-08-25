package actionitems

import (
	"context"
	"time"
)

// Repository persists ActionItems and per-guild configuration.
// Implementations must return ErrNotFound when an ActionItem lookup finds no
// matching row. GetGuildConfig must never return ErrNotFound — an
// unconfigured guild gets a zero-value config with default emotes filled in.
type Repository interface {
	Create(ctx context.Context, item ActionItem) (ActionItem, error)
	Get(ctx context.Context, id string) (ActionItem, error)
	UpdateMessageID(ctx context.Context, id, messageID string) error
	FindPendingByMessageID(ctx context.Context, messageID string) (ActionItem, error)
	SetStatus(ctx context.Context, id string, status Status) error
	Complete(ctx context.Context, id, completedByUserID string, completedAt time.Time, previousStatus Status) error
	ListCompletedSince(ctx context.Context, guildID string, since time.Time, limit int) ([]ActionItem, error)
	Reopen(ctx context.Context, id, newMessageID string, restoreStatus Status) error

	GetGuildConfig(ctx context.Context, guildID string) (GuildConfig, error)
	SetActionItemsChannel(ctx context.Context, guildID, channelID string) error
	SetApproverRole(ctx context.Context, guildID, roleID string) error
	SetEmotes(ctx context.Context, guildID, inProgressEmote, doneEmote string) error
	SetHelpMessageID(ctx context.Context, guildID, messageID string) error

	AddApprover(ctx context.Context, guildID, userID string) error
	RemoveApprover(ctx context.Context, guildID, userID string) error
	ListApprovers(ctx context.Context, guildID string) ([]string, error)
}
