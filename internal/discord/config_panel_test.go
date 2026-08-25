package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func TestConfigPanelContent_Unconfigured(t *testing.T) {
	cfg := actionitems.GuildConfig{InProgressEmote: "🔄", DoneEmote: "✅"}
	got := configPanelContent(cfg)
	want := "**Action Items Configuration**\nChannel: not set\nApprover role: none\nIn-progress emote: 🔄\nDone emote: ✅"
	if got != want {
		t.Errorf("configPanelContent() = %q, want %q", got, want)
	}
}

func TestConfigPanelContent_Configured(t *testing.T) {
	cfg := actionitems.GuildConfig{
		ActionItemsChannelID: "chan1",
		ApproverRoleID:       "role1",
		InProgressEmote:      "👀",
		DoneEmote:            "🎉",
	}
	got := configPanelContent(cfg)
	want := "**Action Items Configuration**\nChannel: <#chan1>\nApprover role: <@&role1>\nIn-progress emote: 👀\nDone emote: 🎉"
	if got != want {
		t.Errorf("configPanelContent() = %q, want %q", got, want)
	}
}

func TestConfigPanelComponents_ThreeRows(t *testing.T) {
	cfg := actionitems.GuildConfig{InProgressEmote: "🔄", DoneEmote: "✅"}
	components := configPanelComponents(cfg)
	if len(components) != 3 {
		t.Fatalf("len(components) = %d, want 3", len(components))
	}

	row0, ok := components[0].(discordgo.ActionsRow)
	if !ok || len(row0.Components) != 1 {
		t.Fatalf("components[0] is not a single-item ActionsRow")
	}
	channelSelect, ok := row0.Components[0].(discordgo.SelectMenu)
	if !ok || channelSelect.MenuType != discordgo.ChannelSelectMenu || channelSelect.CustomID != configChannelSelectCustomID {
		t.Errorf("components[0] channel select = %+v", row0.Components[0])
	}

	row1, ok := components[1].(discordgo.ActionsRow)
	if !ok || len(row1.Components) != 1 {
		t.Fatalf("components[1] is not a single-item ActionsRow")
	}
	roleSelect, ok := row1.Components[0].(discordgo.SelectMenu)
	if !ok || roleSelect.MenuType != discordgo.RoleSelectMenu || roleSelect.CustomID != configRoleSelectCustomID {
		t.Errorf("components[1] role select = %+v", row1.Components[0])
	}
	if roleSelect.MinValues == nil || *roleSelect.MinValues != 0 {
		t.Error("role select MinValues should be 0 so it can be cleared")
	}

	row2, ok := components[2].(discordgo.ActionsRow)
	if !ok || len(row2.Components) != 1 {
		t.Fatalf("components[2] is not a single-item ActionsRow")
	}
	button, ok := row2.Components[0].(discordgo.Button)
	if !ok || button.CustomID != configEditEmotesButtonCustomID {
		t.Errorf("components[2] button = %+v", row2.Components[0])
	}
}

func TestConfigPanelComponents_DefaultValuesWhenConfigured(t *testing.T) {
	cfg := actionitems.GuildConfig{
		ActionItemsChannelID: "chan1",
		ApproverRoleID:       "role1",
		InProgressEmote:      "🔄",
		DoneEmote:            "✅",
	}
	components := configPanelComponents(cfg)

	row0 := components[0].(discordgo.ActionsRow)
	channelSelect := row0.Components[0].(discordgo.SelectMenu)
	if len(channelSelect.DefaultValues) != 1 || channelSelect.DefaultValues[0].ID != "chan1" {
		t.Errorf("channel select default values = %+v", channelSelect.DefaultValues)
	}

	row1 := components[1].(discordgo.ActionsRow)
	roleSelect := row1.Components[0].(discordgo.SelectMenu)
	if len(roleSelect.DefaultValues) != 1 || roleSelect.DefaultValues[0].ID != "role1" {
		t.Errorf("role select default values = %+v", roleSelect.DefaultValues)
	}
}
