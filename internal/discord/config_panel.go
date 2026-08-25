package discord

import (
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const (
	configChannelSelectCustomID    = "config_channel_select"
	configRoleSelectCustomID       = "config_role_select"
	configEditEmotesButtonCustomID = "config_edit_emotes_button"
	configEmotesModalCustomID      = "config_emotes_modal"
	configInProgressEmoteInputID   = "config_in_progress_emote_input"
	configDoneEmoteInputID         = "config_done_emote_input"
)

func configPanelContent(cfg actionitems.GuildConfig) string {
	channel := "not set"
	if cfg.ActionItemsChannelID != "" {
		channel = fmt.Sprintf("<#%s>", cfg.ActionItemsChannelID)
	}
	role := "none"
	if cfg.ApproverRoleID != "" {
		role = fmt.Sprintf("<@&%s>", cfg.ApproverRoleID)
	}
	return fmt.Sprintf(
		"**Action Items Configuration**\nChannel: %s\nApprover role: %s\nIn-progress emote: %s\nDone emote: %s",
		channel, role, cfg.InProgressEmote, cfg.DoneEmote,
	)
}

func configPanelComponents(cfg actionitems.GuildConfig) []discordgo.MessageComponent {
	var channelDefaults []discordgo.SelectMenuDefaultValue
	if cfg.ActionItemsChannelID != "" {
		channelDefaults = append(channelDefaults, discordgo.SelectMenuDefaultValue{
			ID:   cfg.ActionItemsChannelID,
			Type: discordgo.SelectMenuDefaultValueChannel,
		})
	}
	var roleDefaults []discordgo.SelectMenuDefaultValue
	if cfg.ApproverRoleID != "" {
		roleDefaults = append(roleDefaults, discordgo.SelectMenuDefaultValue{
			ID:   cfg.ApproverRoleID,
			Type: discordgo.SelectMenuDefaultValueRole,
		})
	}
	roleMinValues := 0

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:      discordgo.ChannelSelectMenu,
					CustomID:      configChannelSelectCustomID,
					Placeholder:   "Select the action items channel",
					ChannelTypes:  []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
					DefaultValues: channelDefaults,
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:      discordgo.RoleSelectMenu,
					CustomID:      configRoleSelectCustomID,
					Placeholder:   "Select an approver role (optional)",
					MinValues:     &roleMinValues,
					DefaultValues: roleDefaults,
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Edit Emotes",
					Style:    discordgo.SecondaryButton,
					CustomID: configEditEmotesButtonCustomID,
				},
			},
		},
	}
}

func (b *Bot) handleConfigCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to configure this server's action items.")
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to load configuration.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:      discordgo.MessageFlagsEphemeral,
			Content:    configPanelContent(cfg),
			Components: configPanelComponents(cfg),
		},
	})
	if err != nil {
		log.Printf("responding with config panel: %v", err)
	}
}

func (b *Bot) updateConfigPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		return
	}
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    configPanelContent(cfg),
			Components: configPanelComponents(cfg),
		},
	})
	if err != nil {
		log.Printf("updating config panel: %v", err)
	}
}

func (b *Bot) handleConfigChannelSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil || !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to configure this server's action items.")
		return
	}

	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	channelID := values[0]

	if err := b.service.SetActionItemsChannel(ctx, i.GuildID, channelID); err != nil {
		log.Printf("set action items channel: %v", err)
		_ = respondEphemeral(s, i, "Failed to save the channel.")
		return
	}
	if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
		log.Printf("syncing help message: %v", err)
	}
	b.updateConfigPanel(s, i)
}

func (b *Bot) handleConfigRoleSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil || !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to configure this server's action items.")
		return
	}

	roleID := ""
	if values := i.MessageComponentData().Values; len(values) > 0 {
		roleID = values[0]
	}

	if err := b.service.SetApproverRole(ctx, i.GuildID, roleID); err != nil {
		log.Printf("set approver role: %v", err)
		_ = respondEphemeral(s, i, "Failed to save the approver role.")
		return
	}
	if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
		log.Printf("syncing help message: %v", err)
	}
	b.updateConfigPanel(s, i)
}

func (b *Bot) handleConfigEditEmotesButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil || !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to configure this server's action items.")
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to load configuration.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: configEmotesModalCustomID,
			Title:    "Edit Transition Emotes",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  configInProgressEmoteInputID,
							Label:     "In-progress emote",
							Style:     discordgo.TextInputShort,
							Value:     cfg.InProgressEmote,
							Required:  true,
							MaxLength: 32,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  configDoneEmoteInputID,
							Label:     "Done emote",
							Style:     discordgo.TextInputShort,
							Value:     cfg.DoneEmote,
							Required:  true,
							MaxLength: 32,
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("opening emotes modal: %v", err)
	}
}

func (b *Bot) handleConfigEmotesModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil || !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to configure this server's action items.")
		return
	}

	inProgress, done := modalEmoteValues(i.ModalSubmitData().Components)

	if err := b.service.SetEmotes(ctx, i.GuildID, inProgress, done); err != nil {
		log.Printf("set emotes: %v", err)
		_ = respondEphemeral(s, i, "Failed to save emotes. Make sure both fields are filled in.")
		return
	}
	if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
		log.Printf("syncing help message: %v", err)
	}

	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Emotes saved.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    configPanelContent(cfg),
			Components: configPanelComponents(cfg),
		},
	})
	if err != nil {
		log.Printf("updating config panel after emotes: %v", err)
	}
}
