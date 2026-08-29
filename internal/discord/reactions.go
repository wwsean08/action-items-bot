package discord

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func (b *Bot) handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return
	}

	ctx := context.Background()
	item, found, err := b.service.FindPendingByMessage(ctx, r.MessageID)
	if err != nil {
		log.Printf("find pending by message: %v", err)
		return
	}
	if !found {
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, r.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		return
	}

	target, recognized := statusForEmote(cfg, r.Emoji.Name)
	if !recognized {
		return
	}

	member, err := b.resolveMember(s, r.GuildID, r.UserID, r.Member)
	if err != nil {
		log.Printf("fetching guild member: %v", err)
		return
	}

	allowed, err := b.isOwnerOrApprover(ctx, r.GuildID, member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		return
	}
	if !allowed {
		return
	}

	switch target {
	case actionitems.StatusInProgress:
		if item.Status != actionitems.StatusNew {
			return
		}
		if err := b.service.MarkInProgress(ctx, item.ID); err != nil {
			log.Printf("marking in progress: %v", err)
			return
		}
		content := prefixForStatus(actionitems.StatusInProgress) + item.Description
		if _, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:         r.ChannelID,
			ID:              r.MessageID,
			Content:         &content,
			AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
		}); err != nil {
			log.Printf("editing message for in-progress: %v", err)
		}
	case actionitems.StatusDone:
		if err := b.service.CompleteItem(ctx, item.ID, r.UserID, time.Now()); err != nil {
			log.Printf("completing action item: %v", err)
			return
		}
		if err := s.ChannelMessageDelete(r.ChannelID, r.MessageID); err != nil {
			log.Printf("deleting completed action item message: %v", err)
		}
	}
}

func (b *Bot) handleReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return
	}

	ctx := context.Background()
	item, found, err := b.service.FindPendingByMessage(ctx, r.MessageID)
	if err != nil {
		log.Printf("find pending by message: %v", err)
		return
	}
	if !found || item.Status != actionitems.StatusInProgress {
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, r.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		return
	}
	if r.Emoji.Name != cfg.InProgressEmote {
		return
	}

	member, err := b.resolveMember(s, r.GuildID, r.UserID, nil)
	if err != nil {
		log.Printf("fetching guild member: %v", err)
		return
	}

	allowed, err := b.isOwnerOrApprover(ctx, r.GuildID, member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		return
	}
	if !allowed {
		return
	}

	if err := b.service.MarkNew(ctx, item.ID); err != nil {
		log.Printf("marking new: %v", err)
		return
	}
	content := prefixForStatus(actionitems.StatusNew) + item.Description
	if _, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:         r.ChannelID,
		ID:              r.MessageID,
		Content:         &content,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}); err != nil {
		log.Printf("editing message for new: %v", err)
	}
}
