package discord

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func actionItemText(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, opt := range options {
		if opt.Name == "text" {
			return opt.StringValue()
		}
	}
	return ""
}

func undoSelectOptions(items []actionitems.ActionItem) []discordgo.SelectMenuOption {
	options := make([]discordgo.SelectMenuOption, 0, len(items))
	for _, item := range items {
		label := item.Description
		if len(label) > 100 {
			label = label[:97] + "..."
		}
		options = append(options, discordgo.SelectMenuOption{
			Label:       label,
			Value:       item.ID,
			Description: fmt.Sprintf("completed %s", item.CompletedAt.Format(time.RFC822)),
		})
	}
	return options
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
