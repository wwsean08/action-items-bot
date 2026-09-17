package discord

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func newTestBot(t *testing.T, adminIDs ...string) *Bot {
	t.Helper()
	session, err := discordgo.New("Bot faketoken")
	if err != nil {
		t.Fatalf("discordgo.New() error = %v", err)
	}
	admins := make(map[string]struct{}, len(adminIDs))
	for _, id := range adminIDs {
		admins[id] = struct{}{}
	}
	return &Bot{Session: session, botAdminIDs: admins}
}

func TestIsOwnerOrApprover_BotAdminAllowedInAnyGuild(t *testing.T) {
	b := newTestBot(t, "118544113556652032")
	member := &discordgo.Member{User: &discordgo.User{ID: "118544113556652032"}}

	allowed, err := b.isOwnerOrApprover(context.Background(), "some-guild-id", member)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("isOwnerOrApprover() = false, want true for a configured bot admin")
	}
}

func TestIsOwnerOrApprover_NilMemberDeniedBeforeAdminCheck(t *testing.T) {
	b := newTestBot(t)

	allowed, err := b.isOwnerOrApprover(context.Background(), "some-guild-id", nil)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if allowed {
		t.Error("isOwnerOrApprover() = true, want false for a nil member")
	}
}
