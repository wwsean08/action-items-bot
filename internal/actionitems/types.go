package actionitems

import (
	"errors"
	"time"
)

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

const (
	DefaultInProgressEmote = "🔄"
	DefaultDoneEmote       = "✅"
)

type ActionItem struct {
	ID                string
	GuildID           string
	Description       string
	CreatedByUserID   string
	CreatedAt         time.Time
	MessageID         string
	Status            Status
	PreviousStatus    Status
	CompletedByUserID string
	CompletedAt       *time.Time
}

type GuildConfig struct {
	GuildID              string
	ActionItemsChannelID string
	ApproverRoleID       string
	InProgressEmote      string
	DoneEmote            string
	HelpMessageID        string
}

var ErrNotFound = errors.New("action item not found")
