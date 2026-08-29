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

func TestSearchQueryText_ReturnsQueryOptionValue(t *testing.T) {
	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "query", Type: discordgo.ApplicationCommandOptionString, Value: "oat milk"},
	}

	got := searchQueryText(options)

	if got != "oat milk" {
		t.Errorf("searchQueryText = %q, want %q", got, "oat milk")
	}
}

func TestSearchQueryText_ReturnsEmptyWhenMissing(t *testing.T) {
	if got := searchQueryText(nil); got != "" {
		t.Errorf("searchQueryText(nil) = %q, want empty", got)
	}

	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "text", Type: discordgo.ApplicationCommandOptionString, Value: "wrong option"},
	}
	if got := searchQueryText(options); got != "" {
		t.Errorf("searchQueryText(text option) = %q, want empty", got)
	}
}

func TestTruncateDescription(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "shorter than max is unchanged", input: "buy milk", max: 100, want: "buy milk"},
		{name: "exactly max is unchanged", input: strings.Repeat("a", 100), max: 100, want: strings.Repeat("a", 100)},
		{name: "longer is truncated with ellipsis", input: strings.Repeat("a", 150), max: 100, want: strings.Repeat("a", 97) + "..."},
		{name: "empty stays empty", input: "", max: 100, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateDescription(tt.input, tt.max); got != tt.want {
				t.Errorf("truncateDescription() = %q (len %d), want %q (len %d)", got, len(got), tt.want, len(tt.want))
			}
		})
	}
}

func TestSearchResultsText_NoResults(t *testing.T) {
	got := searchResultsText(nil)
	want := "No completed action items matched that search."
	if got != want {
		t.Errorf("searchResultsText(nil) = %q, want %q", got, want)
	}
}

func TestSearchResultsText_SingleResultUsesSingularHeader(t *testing.T) {
	completedAt := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	items := []actionitems.ActionItem{
		{ID: "id1", Description: "buy milk", CompletedAt: &completedAt},
	}

	got := searchResultsText(items)

	if !strings.HasPrefix(got, "Found 1 completed action item:") {
		t.Errorf("searchResultsText() header = %q, want singular header", got)
	}
	if !strings.Contains(got, "- buy milk") {
		t.Errorf("searchResultsText() missing the description in:\n%s", got)
	}
	if !strings.Contains(got, completedAt.Format(time.RFC822)) {
		t.Errorf("searchResultsText() missing the completion time in:\n%s", got)
	}
}

func TestSearchResultsText_MultipleResultsListedInOrder(t *testing.T) {
	newer := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	items := []actionitems.ActionItem{
		{ID: "id1", Description: "newer task", CompletedAt: &newer},
		{ID: "id2", Description: "older task", CompletedAt: &older},
	}

	got := searchResultsText(items)

	if !strings.HasPrefix(got, "Found 2 completed action items:") {
		t.Errorf("searchResultsText() header = %q, want plural header", got)
	}
	newerIdx := strings.Index(got, "newer task")
	olderIdx := strings.Index(got, "older task")
	if newerIdx < 0 || olderIdx < 0 {
		t.Fatalf("searchResultsText() missing a description in:\n%s", got)
	}
	if newerIdx > olderIdx {
		t.Errorf("searchResultsText() reordered results; want input order preserved:\n%s", got)
	}
	if lines := strings.Split(got, "\n"); len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3 (header + 2 results)", len(lines))
	}
}

func TestSearchResultsText_TruncatesLongDescriptions(t *testing.T) {
	completedAt := time.Now()
	items := []actionitems.ActionItem{
		{ID: "id1", Description: strings.Repeat("a", 150), CompletedAt: &completedAt},
	}

	got := searchResultsText(items)

	if strings.Contains(got, strings.Repeat("a", 101)) {
		t.Errorf("searchResultsText() did not truncate a long description:\n%s", got)
	}
	if !strings.Contains(got, strings.Repeat("a", 97)+"...") {
		t.Errorf("searchResultsText() missing truncated description with ellipsis:\n%s", got)
	}
}

func TestSearchResultsText_HandlesMissingCompletedAt(t *testing.T) {
	items := []actionitems.ActionItem{
		{ID: "id1", Description: "buy milk"},
	}

	got := searchResultsText(items)

	if !strings.Contains(got, "- buy milk") {
		t.Errorf("searchResultsText() missing description in:\n%s", got)
	}
	if strings.Contains(got, "(completed") {
		t.Errorf("searchResultsText() should omit the completion time when it is unknown:\n%s", got)
	}
}

func TestSearchResultsText_FitsDiscordMessageLimit(t *testing.T) {
	completedAt := time.Now()
	items := make([]actionitems.ActionItem, 0, 10)
	for n := 0; n < 10; n++ {
		item := actionitems.ActionItem{ID: "id", Description: strings.Repeat("z", 400), CompletedAt: &completedAt}
		items = append(items, item)
	}

	got := searchResultsText(items)

	if len(got) > 2000 {
		t.Errorf("len(searchResultsText()) = %d, want <= 2000 (Discord message limit)", len(got))
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
