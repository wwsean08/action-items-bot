package discord

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const undoSelectCustomID = "undo_select"

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch i.ApplicationCommandData().Name {
		case "action-item":
			b.handleActionItemCommand(s, i)
		case "undo":
			b.handleUndoCommand(s, i)
		case "approver":
			b.handleApproverCommand(s, i)
		case "config":
			b.handleConfigCommand(s, i)
		}
	case discordgo.InteractionMessageComponent:
		switch i.MessageComponentData().CustomID {
		case undoSelectCustomID:
			b.handleUndoSelect(s, i)
		case configChannelSelectCustomID:
			b.handleConfigChannelSelect(s, i)
		case configRoleSelectCustomID:
			b.handleConfigRoleSelect(s, i)
		case configEditEmotesButtonCustomID:
			b.handleConfigEditEmotesButton(s, i)
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == configEmotesModalCustomID {
			b.handleConfigEmotesModalSubmit(s, i)
		}
	}
}

func (b *Bot) handleActionItemCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	guildID := i.GuildID
	text := actionItemText(i.ApplicationCommandData().Options)

	cfg, err := b.service.GetGuildConfig(ctx, guildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to create action item.")
		return
	}
	if cfg.ActionItemsChannelID == "" {
		_ = respondEphemeral(s, i, "This server hasn't configured an action items channel yet. Ask an approver to run /config.")
		return
	}

	item, err := b.service.CreateItem(ctx, guildID, text, i.Member.User.ID, time.Now())
	if err != nil {
		log.Printf("create action item: %v", err)
		_ = respondEphemeral(s, i, "Failed to create action item.")
		return
	}

	posted := prefixForStatus(actionitems.StatusNew) + text
	msg, err := s.ChannelMessageSend(cfg.ActionItemsChannelID, posted)
	if err != nil {
		log.Printf("posting action item message: %v", err)
		_ = respondEphemeral(s, i, "Created, but failed to post to the action items channel.")
		return
	}

	if err := b.service.AttachMessage(ctx, item.ID, msg.ID); err != nil {
		log.Printf("attaching message id: %v", err)
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Action item created: %s", text),
		},
	})
	if err != nil {
		log.Printf("responding to interaction: %v", err)
		return
	}

	reply, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Printf("fetching interaction response: %v", err)
		return
	}
	if err := s.MessageReactionAdd(reply.ChannelID, reply.ID, commandAckEmoji); err != nil {
		log.Printf("reacting to confirmation: %v", err)
	}
}

func (b *Bot) handleUndoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	items, err := b.service.ListUndoable(ctx, i.GuildID, time.Now())
	if err != nil {
		log.Printf("list undoable: %v", err)
		_ = respondEphemeral(s, i, "Failed to look up recent completions.")
		return
	}
	if len(items) == 0 {
		_ = respondEphemeral(s, i, "Nothing to undo.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "Select an action item to restore:",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    undoSelectCustomID,
							Placeholder: "Choose an item to undo",
							Options:     undoSelectOptions(items),
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("responding with undo options: %v", err)
	}
}

func (b *Bot) handleUndoSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	values := i.MessageComponentData().Values
	if len(values) == 0 {
		_ = respondEphemeral(s, i, "No item selected.")
		return
	}
	itemID := values[0]

	item, err := b.service.GetItem(ctx, itemID)
	if err != nil {
		log.Printf("get item for undo: %v", err)
		_ = respondEphemeral(s, i, "That action item could no longer be found.")
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to restore that action item.")
		return
	}

	restoreStatus := item.PreviousStatus
	if restoreStatus == "" {
		restoreStatus = actionitems.StatusNew
	}
	posted := prefixForStatus(restoreStatus) + item.Description
	msg, err := s.ChannelMessageSend(cfg.ActionItemsChannelID, posted)
	if err != nil {
		log.Printf("reposting action item: %v", err)
		_ = respondEphemeral(s, i, "Failed to repost the action item.")
		return
	}

	if err := b.service.UndoItem(ctx, itemID, msg.ID, time.Now()); err != nil {
		log.Printf("undo item: %v", err)
		_ = respondEphemeral(s, i, "Failed to undo that action item.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    fmt.Sprintf("Restored: %s", item.Description),
			Components: []discordgo.MessageComponent{},
		},
	})
	if err != nil {
		log.Printf("confirming undo: %v", err)
	}
}

func (b *Bot) handleApproverCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to manage approvers.")
		return
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		_ = respondEphemeral(s, i, "Missing subcommand.")
		return
	}
	sub := opts[0]

	switch sub.Name {
	case "add":
		userID := subOptionUserID(sub.Options)
		if userID == "" {
			_ = respondEphemeral(s, i, "No user specified.")
			return
		}
		if err := b.service.AddApprover(ctx, i.GuildID, userID); err != nil {
			log.Printf("add approver: %v", err)
			_ = respondEphemeral(s, i, "Failed to add approver.")
			return
		}
		if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
			log.Printf("syncing help message: %v", err)
		}
		_ = respondEphemeral(s, i, fmt.Sprintf("Added <@%s> as an approver.", userID))
	case "remove":
		userID := subOptionUserID(sub.Options)
		if userID == "" {
			_ = respondEphemeral(s, i, "No user specified.")
			return
		}
		if err := b.service.RemoveApprover(ctx, i.GuildID, userID); err != nil {
			log.Printf("remove approver: %v", err)
			_ = respondEphemeral(s, i, "Failed to remove approver.")
			return
		}
		if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
			log.Printf("syncing help message: %v", err)
		}
		_ = respondEphemeral(s, i, fmt.Sprintf("Removed <@%s> as an approver.", userID))
	case "list":
		approvers, err := b.service.ListApprovers(ctx, i.GuildID)
		if err != nil {
			log.Printf("list approvers: %v", err)
			_ = respondEphemeral(s, i, "Failed to list approvers.")
			return
		}
		_ = respondEphemeral(s, i, approverListText(approvers))
	}
}
