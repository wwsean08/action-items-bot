package discord

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func helpMessageBody(cfg actionitems.GuildConfig, ownerID string, approvers []string) string {
	var b strings.Builder
	b.WriteString("**📋 Action Items — How This Works**\n\n")
	b.WriteString("**Creating**: use `/action-item text:\"...\"` to post a new item here.\n\n")
	b.WriteString("**Transitions**:\n")
	fmt.Fprintf(&b, "- React with %s to mark an item **in progress**. Remove the reaction to move it back to new.\n", cfg.InProgressEmote)
	fmt.Fprintf(&b, "- React with %s to mark an item **done**. This removes the message; use `/undo` to bring it back.\n\n", cfg.DoneEmote)

	who := []string{fmt.Sprintf("<@%s> (server owner)", ownerID)}
	if cfg.ApproverRoleID != "" {
		who = append(who, fmt.Sprintf("<@&%s>", cfg.ApproverRoleID))
	}
	for _, id := range approvers {
		who = append(who, fmt.Sprintf("<@%s>", id))
	}
	b.WriteString("**Who can do this**: ")
	b.WriteString(strings.Join(who, ", "))
	b.WriteString("\n\n")

	b.WriteString("**Undo**: if an item was completed by mistake, run `/undo` within 24 hours (last 5 completions) to restore it.\n\n")
	b.WriteString("**Searching**: run `/search query:\"...\"` to look through completed items — results are shown only to you.\n\n")
	b.WriteString("**Configuration**: run `/config` to change the channel, emotes, or approver role. Manage individual approvers with `/approver add`, `/approver remove`, and `/approver list`.")

	return b.String()
}

// syncHelpMessage keeps one pinned explainer message per guild's action
// items channel up to date, editing it in place rather than reposting.
func (b *Bot) syncHelpMessage(ctx context.Context, guildID string) error {
	cfg, err := b.service.GetGuildConfig(ctx, guildID)
	if err != nil {
		return fmt.Errorf("get guild config: %w", err)
	}
	if cfg.ActionItemsChannelID == "" {
		return nil
	}

	guild, err := b.Session.State.Guild(guildID)
	if err != nil {
		guild, err = b.Session.Guild(guildID)
		if err != nil {
			return fmt.Errorf("get guild: %w", err)
		}
	}

	approvers, err := b.service.ListApprovers(ctx, guildID)
	if err != nil {
		return fmt.Errorf("list approvers: %w", err)
	}

	content := helpMessageBody(cfg, guild.OwnerID, approvers)

	if cfg.HelpMessageID != "" {
		if _, err := b.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:         cfg.ActionItemsChannelID,
			ID:              cfg.HelpMessageID,
			Content:         &content,
			AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
		}); err == nil {
			return nil
		}
		log.Printf("editing help message failed for guild %s, reposting", guildID)
	}

	msg, err := b.Session.ChannelMessageSendComplex(cfg.ActionItemsChannelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	})
	if err != nil {
		return fmt.Errorf("posting help message: %w", err)
	}
	if err := b.Session.ChannelMessagePin(cfg.ActionItemsChannelID, msg.ID); err != nil {
		log.Printf("pinning help message: %v", err)
	}
	if err := b.service.SetHelpMessageID(ctx, guildID, msg.ID); err != nil {
		return fmt.Errorf("saving help message id: %w", err)
	}
	return nil
}
