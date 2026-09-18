package discord

import (
	"context"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
	"github.com/wwsean08/action-items-bot/internal/audit"
)

type fakeAuditRepository struct {
	entries []audit.Entry
}

func (f *fakeAuditRepository) Record(_ context.Context, entry audit.Entry) error {
	f.entries = append(f.entries, entry)
	return nil
}

// fakeApproverRepository is a minimal actionitems.Repository whose only
// meaningful behavior backs actionitems.Service.IsApprover (via
// GetGuildConfig and ListApprovers); every other method is unused by these
// tests and returns a zero value.
type fakeApproverRepository struct {
	approverUserIDs []string
}

func (f *fakeApproverRepository) GetGuildConfig(_ context.Context, guildID string) (actionitems.GuildConfig, error) {
	return actionitems.GuildConfig{GuildID: guildID}, nil
}

func (f *fakeApproverRepository) ListApprovers(_ context.Context, _ string) ([]string, error) {
	return f.approverUserIDs, nil
}

func (f *fakeApproverRepository) Create(context.Context, actionitems.ActionItem) (actionitems.ActionItem, error) {
	return actionitems.ActionItem{}, nil
}
func (f *fakeApproverRepository) Get(context.Context, string) (actionitems.ActionItem, error) {
	return actionitems.ActionItem{}, nil
}
func (f *fakeApproverRepository) UpdateMessageID(context.Context, string, string) error { return nil }
func (f *fakeApproverRepository) FindPendingByMessageID(context.Context, string) (actionitems.ActionItem, error) {
	return actionitems.ActionItem{}, nil
}
func (f *fakeApproverRepository) SetStatus(context.Context, string, actionitems.Status) error {
	return nil
}
func (f *fakeApproverRepository) Complete(context.Context, string, string, time.Time, actionitems.Status) error {
	return nil
}
func (f *fakeApproverRepository) ListCompletedSince(context.Context, string, time.Time, int) ([]actionitems.ActionItem, error) {
	return nil, nil
}
func (f *fakeApproverRepository) SearchCompleted(context.Context, string, string, int) ([]actionitems.ActionItem, error) {
	return nil, nil
}
func (f *fakeApproverRepository) Reopen(context.Context, string, string, actionitems.Status) error {
	return nil
}
func (f *fakeApproverRepository) SetActionItemsChannel(context.Context, string, string) error {
	return nil
}
func (f *fakeApproverRepository) SetApproverRole(context.Context, string, string) error { return nil }
func (f *fakeApproverRepository) SetEmotes(context.Context, string, string, string) error {
	return nil
}
func (f *fakeApproverRepository) SetHelpMessageID(context.Context, string, string) error { return nil }
func (f *fakeApproverRepository) AddApprover(context.Context, string, string) error      { return nil }
func (f *fakeApproverRepository) RemoveApprover(context.Context, string, string) error   { return nil }

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

// newTestBotWithApprovers is newTestBot plus a service backed by
// fakeApproverRepository, for tests that need isOwnerOrApprover to reach
// the approver check (anything past the guild-owner check now does).
func newTestBotWithApprovers(t *testing.T, approverUserIDs []string, adminIDs ...string) *Bot {
	t.Helper()
	b := newTestBot(t, adminIDs...)
	b.service = actionitems.NewService(&fakeApproverRepository{approverUserIDs: approverUserIDs})
	return b
}

func TestIsOwnerOrApprover_BotAdminAllowedWhenNotGuildOwnerNorApprover(t *testing.T) {
	b := newTestBotWithApprovers(t, nil, "118544113556652032")
	// Guild state must be seeded even for the bot-admin path now, since the
	// guild-owner check runs first (see TestIsOwnerOrApprover_GuildOwnerTakesPrecedenceOverBotAdmin) —
	// without this, isOwnerOrApprover would fall through to a real network
	// call via b.Session.Guild(guildID) using the fake session's fake token.
	if err := b.Session.State.GuildAdd(&discordgo.Guild{ID: "some-guild-id", OwnerID: "someone-else"}); err != nil {
		t.Fatalf("GuildAdd() error = %v", err)
	}
	member := &discordgo.Member{User: &discordgo.User{ID: "118544113556652032"}}

	allowed, reason, err := b.isOwnerOrApprover(context.Background(), "some-guild-id", member)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("isOwnerOrApprover() allowed = false, want true for a configured bot admin")
	}
	if reason != audit.ReasonBotAdmin {
		t.Errorf("isOwnerOrApprover() reason = %q, want %q", reason, audit.ReasonBotAdmin)
	}
}

func TestIsOwnerOrApprover_NilMemberDeniedBeforeAdminCheck(t *testing.T) {
	b := newTestBot(t)

	allowed, reason, err := b.isOwnerOrApprover(context.Background(), "some-guild-id", nil)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if allowed {
		t.Error("isOwnerOrApprover() allowed = true, want false for a nil member")
	}
	if reason != "" {
		t.Errorf("isOwnerOrApprover() reason = %q, want empty", reason)
	}
}

func TestIsOwnerOrApprover_GuildOwnerAllowed(t *testing.T) {
	b := newTestBot(t)
	if err := b.Session.State.GuildAdd(&discordgo.Guild{ID: "guild1", OwnerID: "owner1"}); err != nil {
		t.Fatalf("GuildAdd() error = %v", err)
	}
	member := &discordgo.Member{User: &discordgo.User{ID: "owner1"}}

	allowed, reason, err := b.isOwnerOrApprover(context.Background(), "guild1", member)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("isOwnerOrApprover() allowed = false, want true for the guild owner")
	}
	if reason != audit.ReasonGuildOwner {
		t.Errorf("isOwnerOrApprover() reason = %q, want %q", reason, audit.ReasonGuildOwner)
	}
}

func TestIsOwnerOrApprover_GuildOwnerTakesPrecedenceOverBotAdmin(t *testing.T) {
	b := newTestBot(t, "owner1") // owner1 is both the guild owner and a configured bot admin
	if err := b.Session.State.GuildAdd(&discordgo.Guild{ID: "guild1", OwnerID: "owner1"}); err != nil {
		t.Fatalf("GuildAdd() error = %v", err)
	}
	member := &discordgo.Member{User: &discordgo.User{ID: "owner1"}}

	allowed, reason, err := b.isOwnerOrApprover(context.Background(), "guild1", member)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("isOwnerOrApprover() allowed = false, want true")
	}
	if reason != audit.ReasonGuildOwner {
		t.Errorf("isOwnerOrApprover() reason = %q, want %q (guild ownership should take precedence over the bot-admin override)", reason, audit.ReasonGuildOwner)
	}
}

func TestIsOwnerOrApprover_ApproverTakesPrecedenceOverBotAdmin(t *testing.T) {
	b := newTestBotWithApprovers(t, []string{"user1"}, "user1") // user1 is both a configured approver and a configured bot admin
	if err := b.Session.State.GuildAdd(&discordgo.Guild{ID: "guild1", OwnerID: "someone-else"}); err != nil {
		t.Fatalf("GuildAdd() error = %v", err)
	}
	member := &discordgo.Member{User: &discordgo.User{ID: "user1"}}

	allowed, reason, err := b.isOwnerOrApprover(context.Background(), "guild1", member)
	if err != nil {
		t.Fatalf("isOwnerOrApprover() error = %v, want nil", err)
	}
	if !allowed {
		t.Error("isOwnerOrApprover() allowed = false, want true")
	}
	if reason != audit.ReasonApprover {
		t.Errorf("isOwnerOrApprover() reason = %q, want %q (approver status should take precedence over the bot-admin override)", reason, audit.ReasonApprover)
	}
}

func TestRecordAudit_WritesEntryWithGivenFields(t *testing.T) {
	fake := &fakeAuditRepository{}
	b := newTestBot(t)
	b.auditLog = fake
	member := &discordgo.Member{User: &discordgo.User{ID: "user1", Username: "alice"}}

	b.recordAudit(context.Background(), "guild1", member, "config.set_channel", audit.ReasonGuildOwner,
		map[string]string{"channel_id": "old"}, map[string]string{"channel_id": "new"})

	if len(fake.entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(fake.entries))
	}
	got := fake.entries[0]
	if got.GuildID != "guild1" || got.UserID != "user1" || got.Username != "alice" {
		t.Errorf("entry = %+v, want guild1/user1/alice", got)
	}
	if got.Action != "config.set_channel" || got.Reason != audit.ReasonGuildOwner {
		t.Errorf("entry action/reason = %q/%q, want config.set_channel/guild_owner", got.Action, got.Reason)
	}
	if got.Before["channel_id"] != "old" || got.After["channel_id"] != "new" {
		t.Errorf("entry before/after = %+v/%+v, want old/new", got.Before, got.After)
	}
	if got.CreatedAt.IsZero() {
		t.Error("entry CreatedAt is zero, want a timestamp")
	}
}

func TestRecordAudit_NoOpWhenAuditLogNil(t *testing.T) {
	b := newTestBot(t) // auditLog left nil
	member := &discordgo.Member{User: &discordgo.User{ID: "user1"}}

	b.recordAudit(context.Background(), "guild1", member, "config.open", audit.ReasonGuildOwner, nil, nil)
	// no panic, nothing to assert beyond that — this is the no-op path
}
