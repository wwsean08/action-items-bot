package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const commandAckEmoji = "✅"

type Bot struct {
	Session *discordgo.Session
	service *actionitems.Service
}

func New(token string, service *actionitems.Service) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuildMembers

	b := &Bot{
		Session: session,
		service: service,
	}
	session.AddHandler(b.handleInteraction)
	session.AddHandler(b.handleReactionAdd)
	session.AddHandler(b.handleReactionRemove)
	return b, nil
}

func (b *Bot) Open() error {
	return b.Session.Open()
}

func (b *Bot) Close() error {
	return b.Session.Close()
}

// commandDefinitions returns the bot's slash command definitions. Kept as a
// package-level function (rather than inline in RegisterCommands) so the
// command set — including deliberate omissions like /search having no
// DefaultMemberPermissions — is plain data a test can assert on.
func commandDefinitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "action-item",
			Description: "Create a new action item",
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
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
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
		},
		{
			Name:        "search",
			Description: "Search completed action items",
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Text to look for in completed action items",
					Required:    true,
				},
			},
		},
		{
			Name:        "approver",
			Description: "Manage who can transition and undo action items",
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Add an approver",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to add", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove an approver",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to remove", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List configured approvers",
				},
			},
		},
		{
			Name:        "config",
			Description: "Open the action items configuration panel",
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
		},
	}
}

// RegisterCommands registers the bot's slash commands globally, so they
// work in any guild the bot is invited to without a per-guild step.
func (b *Bot) RegisterCommands() error {
	for _, cmd := range commandDefinitions() {
		if _, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, "", cmd); err != nil {
			return fmt.Errorf("registering command %s: %w", cmd.Name, err)
		}
	}
	return nil
}
