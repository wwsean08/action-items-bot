package discord

import (
	"strings"
	"testing"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func TestHelpMessageBody_ContainsEmotesAndWhoCanAct(t *testing.T) {
	cfg := actionitems.GuildConfig{
		InProgressEmote: "🔄",
		DoneEmote:       "✅",
		ApproverRoleID:  "role1",
	}
	body := helpMessageBody(cfg, "owner1", []string{"user1", "user2"})

	for _, want := range []string{"🔄", "✅", "<@owner1>", "<@&role1>", "<@user1>", "<@user2>", "/action-item", "/undo", "/search", "/config", "/approver"} {
		if !strings.Contains(body, want) {
			t.Errorf("helpMessageBody() missing %q in:\n%s", want, body)
		}
	}
}

func TestHelpMessageBody_NoRoleOrApproversStillMentionsOwner(t *testing.T) {
	cfg := actionitems.GuildConfig{InProgressEmote: "🔄", DoneEmote: "✅"}
	body := helpMessageBody(cfg, "owner1", nil)

	if !strings.Contains(body, "<@owner1>") {
		t.Errorf("helpMessageBody() missing owner mention in:\n%s", body)
	}
	if strings.Contains(body, "<@&") {
		t.Errorf("helpMessageBody() should not mention a role when none is configured:\n%s", body)
	}
}
