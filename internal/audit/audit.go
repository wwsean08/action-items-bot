// Package audit records which permission path authorized a privileged
// Discord action, for traceability.
package audit

import (
	"context"
	"time"
)

// Reason identifies which permission path granted a privileged action.
type Reason string

const (
	ReasonBotAdmin   Reason = "bot_admin"
	ReasonGuildOwner Reason = "guild_owner"
	ReasonApprover   Reason = "approver"
)

// Entry is one recorded, allowed privileged action.
type Entry struct {
	GuildID   string
	UserID    string
	Username  string
	Action    string
	Reason    Reason
	Before    map[string]string // nil when the action has no meaningful before state
	After     map[string]string // nil when the action has no meaningful after state
	CreatedAt time.Time
}

// Repository persists audit entries.
type Repository interface {
	Record(ctx context.Context, entry Entry) error
}
