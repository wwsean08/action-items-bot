package discord

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/audit"
)

const (
	actionConfigOpen             = "config.open"
	actionConfigSetChannel       = "config.set_channel"
	actionConfigSetRole          = "config.set_role"
	actionConfigEditEmotesButton = "config.edit_emotes_button"
	actionConfigSaveEmotes       = "config.save_emotes"
	actionApproverAdd            = "approver.add"
	actionApproverRemove         = "approver.remove"
	actionApproverList           = "approver.list"
	actionUndoList               = "undo.list"
	actionUndoSelect             = "undo.select"
	actionReactionMarkInProgress = "reaction.mark_in_progress"
	actionReactionMarkDone       = "reaction.mark_done"
	actionReactionMarkNew        = "reaction.mark_new"
)

// isOwnerOrApprover reports whether member is allowed to manage this guild's
// action items configuration or transition/undo items, and if so, which
// permission path granted access: the guild owner (checked live against the
// Discord API), a configured approver, or a configured bot admin (see
// Config.BotAdminIDs) — checked in that order, so a member who holds a
// more specific, genuinely-held role in this guild is attributed as that
// role rather than as bot_admin. Bot admin is a last resort: it only
// applies when the member has no native permission in this particular
// guild. reason is the zero value when allowed is false or err is non-nil.
func (b *Bot) isOwnerOrApprover(ctx context.Context, guildID string, member *discordgo.Member) (bool, audit.Reason, error) {
	if member == nil || member.User == nil {
		return false, "", nil
	}

	guild, err := b.Session.State.Guild(guildID)
	if err != nil {
		guild, err = b.Session.Guild(guildID)
		if err != nil {
			return false, "", err
		}
	}
	if guild.OwnerID == member.User.ID {
		return true, audit.ReasonGuildOwner, nil
	}

	isApprover, err := b.service.IsApprover(ctx, guildID, member.User.ID, member.Roles)
	if err != nil {
		return false, "", err
	}
	if isApprover {
		return true, audit.ReasonApprover, nil
	}

	if _, ok := b.botAdminIDs[member.User.ID]; ok {
		return true, audit.ReasonBotAdmin, nil
	}

	return false, "", nil
}

// recordAudit persists an audit log entry for a privileged action that was
// just allowed. before and after may be nil for actions with no meaningful
// state change (e.g. viewing the config panel). A failure to write is
// logged but never blocks the caller — audit logging is best-effort
// observability, not a permission gate.
func (b *Bot) recordAudit(ctx context.Context, guildID string, member *discordgo.Member, action string, reason audit.Reason, before, after map[string]string) {
	if b.auditLog == nil {
		return
	}
	err := b.auditLog.Record(ctx, audit.Entry{
		GuildID:   guildID,
		UserID:    member.User.ID,
		Username:  member.User.Username,
		Action:    action,
		Reason:    reason,
		Before:    before,
		After:     after,
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("recording audit log entry: %v", err)
	}
}

// resolveMember returns embedded if non-nil (Discord includes it on some
// gateway events), otherwise fetches the member via the REST API.
func (b *Bot) resolveMember(s *discordgo.Session, guildID, userID string, embedded *discordgo.Member) (*discordgo.Member, error) {
	if embedded != nil {
		return embedded, nil
	}
	return s.GuildMember(guildID, userID)
}

// requireOwnerOrApprover checks isOwnerOrApprover and, on denial or error,
// responds to the interaction and returns false alongside a zero Reason.
// Callers should return immediately when the bool is false, and otherwise
// use the returned Reason when calling recordAudit.
func (b *Bot) requireOwnerOrApprover(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, denyMsg string) (bool, audit.Reason) {
	allowed, reason, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return false, ""
	}
	if !allowed {
		_ = respondEphemeral(s, i, denyMsg)
		return false, ""
	}
	return true, reason
}
