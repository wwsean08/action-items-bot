package discord

import (
	"context"
	"log"

	"github.com/bwmarrin/discordgo"
)

// isOwnerOrApprover reports whether member is allowed to manage this guild's
// action items configuration or transition/undo items: either the guild
// owner (checked live against the Discord API), or a configured approver.
func (b *Bot) isOwnerOrApprover(ctx context.Context, guildID string, member *discordgo.Member) (bool, error) {
	if member == nil || member.User == nil {
		return false, nil
	}

	guild, err := b.Session.State.Guild(guildID)
	if err != nil {
		guild, err = b.Session.Guild(guildID)
		if err != nil {
			return false, err
		}
	}
	if guild.OwnerID == member.User.ID {
		return true, nil
	}

	return b.service.IsApprover(ctx, guildID, member.User.ID, member.Roles)
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
// responds to the interaction and returns false. Callers should return
// immediately when this returns false.
func (b *Bot) requireOwnerOrApprover(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, denyMsg string) bool {
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return false
	}
	if !allowed {
		_ = respondEphemeral(s, i, denyMsg)
		return false
	}
	return true
}
