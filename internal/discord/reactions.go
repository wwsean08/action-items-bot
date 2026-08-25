package discord

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
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

	member := r.Member
	if member == nil {
		m, err := s.GuildMember(r.GuildID, r.UserID)
		if err != nil {
			log.Printf("fetching guild member: %v", err)
			return
		}
		member = m
	}
	if !b.isApprover(member) {
		return
	}

	if err := s.ChannelMessageDelete(r.ChannelID, r.MessageID); err != nil {
		log.Printf("deleting completed action item message: %v", err)
	}
	if err := b.service.CompleteItem(ctx, item.ID, r.UserID, time.Now()); err != nil {
		log.Printf("completing action item: %v", err)
	}
}
