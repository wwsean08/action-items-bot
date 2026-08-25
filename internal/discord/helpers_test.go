package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func TestActionItemText_ReturnsTextOptionValue(t *testing.T) {
	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "text", Type: discordgo.ApplicationCommandOptionString, Value: "buy milk"},
	}

	got := actionItemText(options)

	if got != "buy milk" {
		t.Errorf("actionItemText = %q, want %q", got, "buy milk")
	}
}

func TestActionItemText_ReturnsEmptyWhenMissing(t *testing.T) {
	got := actionItemText(nil)

	if got != "" {
		t.Errorf("actionItemText = %q, want empty", got)
	}
}

func TestUndoSelectOptions_MapsItemsToOptions(t *testing.T) {
	completedAt := time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)
	items := []actionitems.ActionItem{
		{ID: "id1", Description: "buy milk", CompletedAt: &completedAt},
	}

	options := undoSelectOptions(items)

	if len(options) != 1 {
		t.Fatalf("len(options) = %d, want 1", len(options))
	}
	if options[0].Value != "id1" {
		t.Errorf("Value = %q, want %q", options[0].Value, "id1")
	}
	if options[0].Label != "buy milk" {
		t.Errorf("Label = %q, want %q", options[0].Label, "buy milk")
	}
}

func TestUndoSelectOptions_TruncatesLongDescriptions(t *testing.T) {
	completedAt := time.Now()
	longDescription := strings.Repeat("a", 150)
	items := []actionitems.ActionItem{
		{ID: "id1", Description: longDescription, CompletedAt: &completedAt},
	}

	options := undoSelectOptions(items)

	if len(options[0].Label) != 100 {
		t.Errorf("len(Label) = %d, want 100", len(options[0].Label))
	}
	if !strings.HasSuffix(options[0].Label, "...") {
		t.Errorf("Label = %q, want suffix '...'", options[0].Label)
	}
}

func TestPrefixForStatus(t *testing.T) {
	tests := []struct {
		status actionitems.Status
		want   string
	}{
		{actionitems.StatusNew, "[NEW] "},
		{actionitems.StatusInProgress, "[IN PROGRESS] "},
		{actionitems.StatusDone, "[DONE] "},
	}
	for _, tt := range tests {
		if got := prefixForStatus(tt.status); got != tt.want {
			t.Errorf("prefixForStatus(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestStatusForEmote(t *testing.T) {
	cfg := actionitems.GuildConfig{InProgressEmote: "🔄", DoneEmote: "✅"}

	status, ok := statusForEmote(cfg, "🔄")
	if !ok || status != actionitems.StatusInProgress {
		t.Errorf("statusForEmote(in-progress emote) = %q, %v, want StatusInProgress, true", status, ok)
	}

	status, ok = statusForEmote(cfg, "✅")
	if !ok || status != actionitems.StatusDone {
		t.Errorf("statusForEmote(done emote) = %q, %v, want StatusDone, true", status, ok)
	}

	_, ok = statusForEmote(cfg, "🍕")
	if ok {
		t.Error("statusForEmote(unrecognized emote) = true, want false")
	}
}

func TestSubOptionUserID(t *testing.T) {
	opts := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "user", Value: "12345"},
	}
	if got := subOptionUserID(opts); got != "12345" {
		t.Errorf("subOptionUserID() = %q, want %q", got, "12345")
	}

	if got := subOptionUserID(nil); got != "" {
		t.Errorf("subOptionUserID(nil) = %q, want empty", got)
	}
}

func TestApproverListText_Empty(t *testing.T) {
	got := approverListText(nil)
	want := "No approvers configured yet."
	if got != want {
		t.Errorf("approverListText(nil) = %q, want %q", got, want)
	}
}

func TestApproverListText_WithApprovers(t *testing.T) {
	got := approverListText([]string{"user1", "user2"})
	want := "Approvers:\n- <@user1>\n- <@user2>"
	if got != want {
		t.Errorf("approverListText() = %q, want %q", got, want)
	}
}

func TestModalEmoteValues(t *testing.T) {
	components := []discordgo.MessageComponent{
		&discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: configInProgressEmoteInputID, Value: "👀"},
			},
		},
		&discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: configDoneEmoteInputID, Value: "🎉"},
			},
		},
	}

	inProgress, done := modalEmoteValues(components)
	if inProgress != "👀" {
		t.Errorf("inProgress = %q, want %q", inProgress, "👀")
	}
	if done != "🎉" {
		t.Errorf("done = %q, want %q", done, "🎉")
	}
}
