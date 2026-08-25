package discord

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
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
		}
	case discordgo.InteractionMessageComponent:
		if i.MessageComponentData().CustomID == undoSelectCustomID {
			b.handleUndoSelect(s, i)
		}
	}
}

func (b *Bot) handleActionItemCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	text := actionItemText(i.ApplicationCommandData().Options)

	item, err := b.service.CreateItem(ctx, text, i.Member.User.ID, time.Now())
	if err != nil {
		log.Printf("create action item: %v", err)
		_ = respondEphemeral(s, i, "Failed to create action item.")
		return
	}

	msg, err := s.ChannelMessageSend(b.actionItemsChannelID, text)
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
	if err := s.MessageReactionAdd(reply.ChannelID, reply.ID, doneEmoji); err != nil {
		log.Printf("reacting to confirmation: %v", err)
	}
}

func (b *Bot) handleUndoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.isApprover(i.Member) {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	items, err := b.service.ListUndoable(context.Background(), time.Now())
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
	if !b.isApprover(i.Member) {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	values := i.MessageComponentData().Values
	if len(values) == 0 {
		_ = respondEphemeral(s, i, "No item selected.")
		return
	}
	itemID := values[0]
	ctx := context.Background()

	item, err := b.service.GetItem(ctx, itemID)
	if err != nil {
		log.Printf("get item for undo: %v", err)
		_ = respondEphemeral(s, i, "That action item could no longer be found.")
		return
	}

	msg, err := s.ChannelMessageSend(b.actionItemsChannelID, item.Description)
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
