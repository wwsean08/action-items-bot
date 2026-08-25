package actionitems

import (
	"errors"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
)

type ActionItem struct {
	ID                string
	Description       string
	CreatedByUserID   string
	CreatedAt         time.Time
	MessageID         string
	Status            Status
	CompletedByUserID string
	CompletedAt       *time.Time
}

var ErrNotFound = errors.New("action item not found")
