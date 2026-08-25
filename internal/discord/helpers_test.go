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
