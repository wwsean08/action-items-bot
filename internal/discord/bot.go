package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const doneEmoji = "✅" // ✅

type Bot struct {
	Session              *discordgo.Session
	service              *actionitems.Service
	approvers            actionitems.ApproverChecker
	guildID              string
	actionItemsChannelID string
}

func New(token string, service *actionitems.Service, approvers actionitems.ApproverChecker, guildID, actionItemsChannelID string) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuildMembers

	b := &Bot{
		Session:              session,
		service:              service,
		approvers:            approvers,
		guildID:              guildID,
		actionItemsChannelID: actionItemsChannelID,
	}
	session.AddHandler(b.handleInteraction)
	session.AddHandler(b.handleReactionAdd)
	return b, nil
}

func (b *Bot) Open() error {
	return b.Session.Open()
}

func (b *Bot) Close() error {
	return b.Session.Close()
}

// RegisterCommands registers the /action-item and /undo slash commands for
// the configured guild. Must be called after Open.
func (b *Bot) RegisterCommands() error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "action-item",
			Description: "Create a new action item",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "text",
					Description: "The action item description",
					Required:    true,
				},
			},
		},
		{
			Name:        "undo",
			Description: "Undo a recently completed action item",
		},
	}

	for _, cmd := range commands {
		if _, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, b.guildID, cmd); err != nil {
			return fmt.Errorf("registering command %s: %w", cmd.Name, err)
		}
	}
	return nil
}

func (b *Bot) isApprover(member *discordgo.Member) bool {
	if member == nil || member.User == nil {
		return false
	}
	return b.approvers.IsApprover(member.User.ID, member.Roles)
}
