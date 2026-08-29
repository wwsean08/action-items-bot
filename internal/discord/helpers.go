package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

// maxDisplayedDescription bounds how much of a description is echoed back in
// select-menu labels and search results, keeping responses well under
// Discord's 2000-character message limit.
const maxDisplayedDescription = 100

func actionItemText(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, opt := range options {
		if opt.Name == "text" {
			return opt.StringValue()
		}
	}
	return ""
}

func searchQueryText(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, opt := range options {
		if opt.Name == "query" {
			return opt.StringValue()
		}
	}
	return ""
}

func undoSelectOptions(items []actionitems.ActionItem) []discordgo.SelectMenuOption {
	options := make([]discordgo.SelectMenuOption, 0, len(items))
	for _, item := range items {
		options = append(options, discordgo.SelectMenuOption{
			Label:       truncateDescription(item.Description, maxDisplayedDescription),
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

func prefixForStatus(status actionitems.Status) string {
	switch status {
	case actionitems.StatusInProgress:
		return "[IN PROGRESS] "
	case actionitems.StatusDone:
		return "[DONE] "
	default:
		return "[NEW] "
	}
}

func statusForEmote(cfg actionitems.GuildConfig, emojiName string) (actionitems.Status, bool) {
	switch emojiName {
	case cfg.InProgressEmote:
		return actionitems.StatusInProgress, true
	case cfg.DoneEmote:
		return actionitems.StatusDone, true
	default:
		return "", false
	}
}

func subOptionUserID(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, opt := range options {
		if opt.Name == "user" {
			if id, ok := opt.Value.(string); ok {
				return id
			}
		}
	}
	return ""
}

func approverListText(approvers []string) string {
	if len(approvers) == 0 {
		return "No approvers configured yet."
	}
	lines := make([]string, 0, len(approvers)+1)
	lines = append(lines, "Approvers:")
	for _, id := range approvers {
		lines = append(lines, fmt.Sprintf("- <@%s>", id))
	}
	return strings.Join(lines, "\n")
}

func truncateDescription(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func searchResultsText(items []actionitems.ActionItem) string {
	if len(items) == 0 {
		return "No completed action items matched that search."
	}

	lines := make([]string, 0, len(items)+1)
	if len(items) == 1 {
		lines = append(lines, "Found 1 completed action item:")
	} else {
		lines = append(lines, fmt.Sprintf("Found %d completed action items:", len(items)))
	}
	for _, item := range items {
		description := truncateDescription(item.Description, maxDisplayedDescription)
		if item.CompletedAt == nil {
			lines = append(lines, fmt.Sprintf("- %s", description))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (completed %s)", description, item.CompletedAt.Format(time.RFC822)))
	}
	return strings.Join(lines, "\n")
}

func modalEmoteValues(components []discordgo.MessageComponent) (inProgress, done string) {
	for _, comp := range components {
		row, ok := comp.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, inner := range row.Components {
			input, ok := inner.(*discordgo.TextInput)
			if !ok {
				continue
			}
			switch input.CustomID {
			case configInProgressEmoteInputID:
				inProgress = input.Value
			case configDoneEmoteInputID:
				done = input.Value
			}
		}
	}
	return inProgress, done
}
