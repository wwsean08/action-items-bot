package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestCommandDefinitions_SearchHasNoPermissionRestriction(t *testing.T) {
	var search *discordgo.ApplicationCommand
	for _, cmd := range commandDefinitions() {
		if cmd.Name == "search" {
			search = cmd
			break
		}
	}
	if search == nil {
		t.Fatal(`commandDefinitions() has no "search" command`)
	}

	if search.DefaultMemberPermissions != nil {
		t.Errorf("search command DefaultMemberPermissions = %v, want nil (access is governed by each guild's own Integrations settings, not the bot)", *search.DefaultMemberPermissions)
	}

	if len(search.Options) != 1 {
		t.Fatalf("search command has %d options, want 1", len(search.Options))
	}
	queryOption := search.Options[0]
	if queryOption.Name != "query" {
		t.Errorf("search command's option name = %q, want %q", queryOption.Name, "query")
	}
	if !queryOption.Required {
		t.Error("search command's query option should be required")
	}
}
