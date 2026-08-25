# Discord Action Items Bot — Multi-Tenant + State Machine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the single-tenant, env-var-configured Discord action items bot into a multi-tenant bot with a three-state (`new`/`in_progress`/`done`) item lifecycle, per-guild configuration managed via Discord commands, and a self-updating pinned help message.

**Architecture:** Guild-specific config (channel, approvers, emotes) moves from environment variables into two new Postgres tables (`guild_configs`, `approvers`), managed through new `/config` (interactive panel) and `/approver` commands. `action_items` gains `guild_id` and `previous_status` columns. The existing discord → actionitems service → postgres repository layering is preserved; only the layers' internals change.

**Tech Stack:** Go 1.26, discordgo v0.29.0, pgx/v5, golang-migrate/v4, Postgres 16.

**Spec:** `docs/superpowers/specs/2026-08-24-multi-tenant-state-machine-design.md`

## Global Constraints

- Slash commands register **globally** (`ApplicationCommandCreate(appID, "", cmd)`), not per-guild.
- Every write-command handler (`/config`, `/approver`, their components/modal) must call `isOwnerOrApprover` before mutating anything.
- State values are exactly `"new"`, `"in_progress"`, `"done"` (lowercase, snake_case) — stored as plain `text`, no DB constraint.
- Default emotes: in-progress `🔄`, done `✅` — defined once as `actionitems.DefaultInProgressEmote` / `actionitems.DefaultDoneEmote` and reused everywhere a default is needed.
- Message prefixes: `"[NEW] "`, `"[IN PROGRESS] "` (trailing space included, prepended directly to the description). `done` has no prefix since the message is deleted.
- Keep the existing testing philosophy: business logic (service layer, pure discord-layer helpers) gets full TDD unit tests against the fake repository or in isolation; Discord session glue code (anything that calls `s.Session.*` directly) stays thin and untested, verified only by `go build`/`go vet` and manual checks — this matches the codebase's existing convention (see `internal/discord/commands.go`, `reactions.go` today).
- All new/changed repository methods on `postgres.Repository` must keep the compile-time check `var _ actionitems.Repository = (*Repository)(nil)` passing.

---

### Task 1: Shrink environment configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config{DiscordToken, DatabaseURL, HealthPort string}`, `config.Load(getenv func(string) string) (Config, error)` — same signature as today, smaller struct.

- [ ] **Step 1: Read the current test file to see the existing table-driven cases**

Run: `cat internal/config/config_test.go`

Confirm the existing tests reference `TestLoad_ValidConfig`, `TestLoad_MissingRequiredFields`, `TestLoad_NoApproversConfigured`, `TestLoad_ApproverRoleIDAloneIsSufficient`, `TestLoad_HealthPortDefaultsTo8080`, `TestLoad_HealthPortOverride`, and a `validEnv()`/`loadWithEnv()` helper pair.

- [ ] **Step 2: Rewrite the failing tests first**

Replace the whole file with:

```go
package config

import "testing"

func validEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN": "token123",
		"DATABASE_URL":  "postgres://localhost/action_items",
	}
}

func loadWithEnv(env map[string]string) (Config, error) {
	return Load(func(key string) string {
		return env[key]
	})
}

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.DiscordToken != "token123" {
		t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "token123")
	}
	if cfg.DatabaseURL != "postgres://localhost/action_items" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/action_items")
	}
	if cfg.HealthPort != "8080" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "8080")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		missing string
	}{
		{name: "missing discord token", missing: "DISCORD_TOKEN"},
		{name: "missing database url", missing: "DATABASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			delete(env, tt.missing)

			_, err := loadWithEnv(env)
			if err == nil {
				t.Fatalf("Load() error = nil, want an error about missing %s", tt.missing)
			}
		})
	}
}

func TestLoad_HealthPortDefaultsTo8080(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.HealthPort != "8080" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "8080")
	}
}

func TestLoad_HealthPortOverride(t *testing.T) {
	env := validEnv()
	env["HEALTH_PORT"] = "9090"

	cfg, err := loadWithEnv(env)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.HealthPort != "9090" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "9090")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail to compile against the old Config struct**

Run: `go test ./internal/config/...`
Expected: FAIL — compile error, since `cfg.DiscordToken`/`cfg.DatabaseURL`/`cfg.HealthPort` still exist but the old required-field tests referencing `GUILD_ID` etc. are gone from this file while `config.go` still requires them, so `TestLoad_ValidConfig` fails at runtime with "GUILD_ID is required".

- [ ] **Step 4: Rewrite the implementation**

```go
package config

import "fmt"

type Config struct {
	DiscordToken string
	DatabaseURL  string
	HealthPort   string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DiscordToken: getenv("DISCORD_TOKEN"),
		DatabaseURL:  getenv("DATABASE_URL"),
		HealthPort:   getenv("HEALTH_PORT"),
	}
	if cfg.HealthPort == "" {
		cfg.HealthPort = "8080"
	}

	if cfg.DiscordToken == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS, all 4 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: shrink env config to token/database/health-port for multi-tenant"
```

---

### Task 2: Rebuild the actionitems domain layer

**Files:**
- Modify: `internal/actionitems/types.go`
- Modify: `internal/actionitems/repository.go`
- Modify: `internal/actionitems/approver.go`
- Modify: `internal/actionitems/approver_test.go`
- Modify: `internal/actionitems/fake_repository_test.go`
- Modify: `internal/actionitems/service.go`
- Modify: `internal/actionitems/service_test.go`

**Interfaces:**
- Produces (consumed by discord layer in later tasks and postgres layer in Task 3):
  - `actionitems.Status` = `StatusNew | StatusInProgress | StatusDone` (string consts `"new"`, `"in_progress"`, `"done"`)
  - `actionitems.DefaultInProgressEmote = "🔄"`, `actionitems.DefaultDoneEmote = "✅"`
  - `actionitems.ActionItem{ID, GuildID, Description, CreatedByUserID, CreatedAt, MessageID, Status, PreviousStatus, CompletedByUserID, CompletedAt}`
  - `actionitems.GuildConfig{GuildID, ActionItemsChannelID, ApproverRoleID, InProgressEmote, DoneEmote, HelpMessageID}`
  - `actionitems.ErrNotFound`, `actionitems.ErrNotUndoable`, `actionitems.ErrInvalidTransition`, `actionitems.ErrInvalidEmote`
  - `Service.CreateItem(ctx, guildID, description, createdByUserID string, now time.Time) (ActionItem, error)`
  - `Service.AttachMessage(ctx, id, messageID string) error`
  - `Service.FindPendingByMessage(ctx, messageID string) (ActionItem, bool, error)`
  - `Service.GetItem(ctx, id string) (ActionItem, error)`
  - `Service.MarkInProgress(ctx, id string) error`
  - `Service.MarkNew(ctx, id string) error`
  - `Service.CompleteItem(ctx, id, completedByUserID string, now time.Time) error`
  - `Service.ListUndoable(ctx, guildID string, now time.Time) ([]ActionItem, error)`
  - `Service.UndoItem(ctx, id, newMessageID string, now time.Time) error`
  - `Service.GetGuildConfig(ctx, guildID string) (GuildConfig, error)`
  - `Service.SetActionItemsChannel(ctx, guildID, channelID string) error`
  - `Service.SetApproverRole(ctx, guildID, roleID string) error`
  - `Service.SetEmotes(ctx, guildID, inProgressEmote, doneEmote string) error`
  - `Service.SetHelpMessageID(ctx, guildID, messageID string) error`
  - `Service.AddApprover(ctx, guildID, userID string) error`
  - `Service.RemoveApprover(ctx, guildID, userID string) error`
  - `Service.ListApprovers(ctx, guildID string) ([]string, error)`
  - `Service.IsApprover(ctx, guildID, userID string, memberRoleIDs []string) (bool, error)`

This task rewrites `types.go`, `repository.go`, `approver.go`, and `fake_repository_test.go` directly (they're test infrastructure and shared type definitions — not independently TDD'd, same as the existing codebase's convention where `fake_repository_test.go` has no dedicated tests of its own). Then it TDDs `approver.go`'s pure matcher and every `Service` method against the fake.

- [ ] **Step 1: Rewrite `types.go`**

```go
package actionitems

import (
	"errors"
	"time"
)

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

const (
	DefaultInProgressEmote = "🔄"
	DefaultDoneEmote       = "✅"
)

type ActionItem struct {
	ID                 string
	GuildID            string
	Description        string
	CreatedByUserID    string
	CreatedAt          time.Time
	MessageID          string
	Status             Status
	PreviousStatus     Status
	CompletedByUserID  string
	CompletedAt        *time.Time
}

type GuildConfig struct {
	GuildID              string
	ActionItemsChannelID string
	ApproverRoleID       string
	InProgressEmote      string
	DoneEmote            string
	HelpMessageID        string
}

var ErrNotFound = errors.New("action item not found")
```

- [ ] **Step 2: Rewrite `repository.go`**

```go
package actionitems

import (
	"context"
	"time"
)

// Repository persists ActionItems and per-guild configuration.
// Implementations must return ErrNotFound when an ActionItem lookup finds no
// matching row. GetGuildConfig must never return ErrNotFound — an
// unconfigured guild gets a zero-value config with default emotes filled in.
type Repository interface {
	Create(ctx context.Context, item ActionItem) (ActionItem, error)
	Get(ctx context.Context, id string) (ActionItem, error)
	UpdateMessageID(ctx context.Context, id, messageID string) error
	FindPendingByMessageID(ctx context.Context, messageID string) (ActionItem, error)
	SetStatus(ctx context.Context, id string, status Status) error
	Complete(ctx context.Context, id, completedByUserID string, completedAt time.Time, previousStatus Status) error
	ListCompletedSince(ctx context.Context, guildID string, since time.Time, limit int) ([]ActionItem, error)
	Reopen(ctx context.Context, id, newMessageID string, restoreStatus Status) error

	GetGuildConfig(ctx context.Context, guildID string) (GuildConfig, error)
	SetActionItemsChannel(ctx context.Context, guildID, channelID string) error
	SetApproverRole(ctx context.Context, guildID, roleID string) error
	SetEmotes(ctx context.Context, guildID, inProgressEmote, doneEmote string) error
	SetHelpMessageID(ctx context.Context, guildID, messageID string) error

	AddApprover(ctx context.Context, guildID, userID string) error
	RemoveApprover(ctx context.Context, guildID, userID string) error
	ListApprovers(ctx context.Context, guildID string) ([]string, error)
}
```

- [ ] **Step 3: Rewrite `fake_repository_test.go`**

```go
package actionitems

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// fakeRepository is an in-memory Repository used only in tests.
type fakeRepository struct {
	items     map[string]ActionItem
	nextID    int
	configs   map[string]GuildConfig
	approvers map[string]map[string]bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		items:     make(map[string]ActionItem),
		configs:   make(map[string]GuildConfig),
		approvers: make(map[string]map[string]bool),
	}
}

func (f *fakeRepository) Create(_ context.Context, item ActionItem) (ActionItem, error) {
	f.nextID++
	item.ID = strconv.Itoa(f.nextID)
	f.items[item.ID] = item
	return item, nil
}

func (f *fakeRepository) Get(_ context.Context, id string) (ActionItem, error) {
	item, ok := f.items[id]
	if !ok {
		return ActionItem{}, ErrNotFound
	}
	return item, nil
}

func (f *fakeRepository) UpdateMessageID(_ context.Context, id, messageID string) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.MessageID = messageID
	f.items[id] = item
	return nil
}

func (f *fakeRepository) FindPendingByMessageID(_ context.Context, messageID string) (ActionItem, error) {
	for _, item := range f.items {
		if item.MessageID == messageID && item.Status != StatusDone {
			return item, nil
		}
	}
	return ActionItem{}, ErrNotFound
}

func (f *fakeRepository) SetStatus(_ context.Context, id string, status Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = status
	f.items[id] = item
	return nil
}

func (f *fakeRepository) Complete(_ context.Context, id, completedByUserID string, completedAt time.Time, previousStatus Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = StatusDone
	item.PreviousStatus = previousStatus
	item.CompletedByUserID = completedByUserID
	ca := completedAt
	item.CompletedAt = &ca
	f.items[id] = item
	return nil
}

func (f *fakeRepository) ListCompletedSince(_ context.Context, guildID string, since time.Time, limit int) ([]ActionItem, error) {
	var result []ActionItem
	for _, item := range f.items {
		if item.GuildID == guildID && item.Status == StatusDone && item.CompletedAt != nil && !item.CompletedAt.Before(since) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CompletedAt.After(*result[j].CompletedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepository) Reopen(_ context.Context, id, newMessageID string, restoreStatus Status) error {
	item, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = restoreStatus
	item.MessageID = newMessageID
	item.CompletedByUserID = ""
	item.CompletedAt = nil
	item.PreviousStatus = ""
	f.items[id] = item
	return nil
}

func (f *fakeRepository) getOrCreateConfig(guildID string) GuildConfig {
	cfg, ok := f.configs[guildID]
	if !ok {
		cfg = GuildConfig{GuildID: guildID, InProgressEmote: DefaultInProgressEmote, DoneEmote: DefaultDoneEmote}
	}
	return cfg
}

func (f *fakeRepository) GetGuildConfig(_ context.Context, guildID string) (GuildConfig, error) {
	return f.getOrCreateConfig(guildID), nil
}

func (f *fakeRepository) SetActionItemsChannel(_ context.Context, guildID, channelID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.ActionItemsChannelID = channelID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetApproverRole(_ context.Context, guildID, roleID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.ApproverRoleID = roleID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetEmotes(_ context.Context, guildID, inProgressEmote, doneEmote string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.InProgressEmote = inProgressEmote
	cfg.DoneEmote = doneEmote
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) SetHelpMessageID(_ context.Context, guildID, messageID string) error {
	cfg := f.getOrCreateConfig(guildID)
	cfg.HelpMessageID = messageID
	f.configs[guildID] = cfg
	return nil
}

func (f *fakeRepository) AddApprover(_ context.Context, guildID, userID string) error {
	if f.approvers[guildID] == nil {
		f.approvers[guildID] = make(map[string]bool)
	}
	f.approvers[guildID][userID] = true
	return nil
}

func (f *fakeRepository) RemoveApprover(_ context.Context, guildID, userID string) error {
	delete(f.approvers[guildID], userID)
	return nil
}

func (f *fakeRepository) ListApprovers(_ context.Context, guildID string) ([]string, error) {
	var result []string
	for userID := range f.approvers[guildID] {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result, nil
}
```

- [ ] **Step 4: Verify the package compiles with just these three files (service.go/approver.go still stale)**

Run: `go build ./internal/actionitems/...`
Expected: FAIL — `service.go` and `approver.go` still reference the old API (e.g. `StatusPending`, `ApproverChecker`, old method signatures). This is expected; both are rewritten in the next steps.

- [ ] **Step 5: Write the failing test for the pure approver-matching helper**

Replace `internal/actionitems/approver_test.go` with:

```go
package actionitems

import "testing"

func TestIsApproverMatch_UserInList(t *testing.T) {
	got := isApproverMatch("user1", nil, []string{"user1", "user2"}, "")
	if !got {
		t.Error("expected user1 to match")
	}
}

func TestIsApproverMatch_UserNotInListNoRole(t *testing.T) {
	got := isApproverMatch("user3", nil, []string{"user1", "user2"}, "")
	if got {
		t.Error("expected user3 not to match")
	}
}

func TestIsApproverMatch_RoleMatch(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-a", "role-b"}, nil, "role-a")
	if !got {
		t.Error("expected role-a to match")
	}
}

func TestIsApproverMatch_NoRoleConfiguredIgnoresMemberRoles(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-a"}, nil, "")
	if got {
		t.Error("expected no match when no approver role is configured")
	}
}

func TestIsApproverMatch_NeitherUserNorRoleMatches(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-b"}, []string{"user1"}, "role-a")
	if got {
		t.Error("expected no match")
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/actionitems/... -run TestIsApproverMatch -v`
Expected: FAIL — `isApproverMatch` undefined.

- [ ] **Step 7: Rewrite `approver.go`**

```go
package actionitems

// isApproverMatch reports whether a user is an approver: either directly
// listed in approverUserIDs, or a member of approverRoleID (when configured).
func isApproverMatch(userID string, memberRoleIDs, approverUserIDs []string, approverRoleID string) bool {
	for _, id := range approverUserIDs {
		if id == userID {
			return true
		}
	}
	if approverRoleID == "" {
		return false
	}
	for _, role := range memberRoleIDs {
		if role == approverRoleID {
			return true
		}
	}
	return false
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/actionitems/... -run TestIsApproverMatch -v`
Expected: PASS, all 5 cases. (The package as a whole still won't build — `service.go` is next.)

- [ ] **Step 9: Write the failing tests for the rewritten service**

Replace `internal/actionitems/service_test.go` with:

```go
package actionitems

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestService() *Service {
	return NewService(newFakeRepository())
}

func TestCreateItem_SetsStatusNewAndGuild(t *testing.T) {
	s := newTestService()
	now := time.Now()

	item, err := s.CreateItem(context.Background(), "guild1", "buy milk", "user1", now)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if item.Status != StatusNew {
		t.Errorf("Status = %q, want %q", item.Status, StatusNew)
	}
	if item.GuildID != "guild1" {
		t.Errorf("GuildID = %q, want %q", item.GuildID, "guild1")
	}
	if item.ID == "" {
		t.Error("expected an ID to be assigned")
	}
}

func TestFindPendingByMessage_MatchesNewAndInProgressNotDone(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	newItem, _ := s.CreateItem(ctx, "guild1", "new item", "user1", now)
	_ = s.AttachMessage(ctx, newItem.ID, "msg-new")

	inProgressItem, _ := s.CreateItem(ctx, "guild1", "in progress item", "user1", now)
	_ = s.AttachMessage(ctx, inProgressItem.ID, "msg-in-progress")
	_ = s.MarkInProgress(ctx, inProgressItem.ID)

	doneItem, _ := s.CreateItem(ctx, "guild1", "done item", "user1", now)
	_ = s.AttachMessage(ctx, doneItem.ID, "msg-done")
	_ = s.CompleteItem(ctx, doneItem.ID, "approver1", now)

	if _, found, _ := s.FindPendingByMessage(ctx, "msg-new"); !found {
		t.Error("expected to find the new item")
	}
	if _, found, _ := s.FindPendingByMessage(ctx, "msg-in-progress"); !found {
		t.Error("expected to find the in-progress item")
	}
	if _, found, _ := s.FindPendingByMessage(ctx, "msg-done"); found {
		t.Error("expected not to find the done item")
	}
}

func TestMarkInProgress_FromNewSucceeds(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())

	if err := s.MarkInProgress(ctx, item.ID); err != nil {
		t.Fatalf("MarkInProgress() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
}

func TestMarkInProgress_FromInProgressFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	err := s.MarkInProgress(ctx, item.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestMarkNew_FromInProgressSucceeds(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	if err := s.MarkNew(ctx, item.ID); err != nil {
		t.Fatalf("MarkNew() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusNew {
		t.Errorf("Status = %q, want %q", got.Status, StatusNew)
	}
}

func TestMarkNew_FromNewFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())

	err := s.MarkNew(ctx, item.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestCompleteItem_FromNewRecordsPreviousStatusNew(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	now := time.Now()

	if err := s.CompleteItem(ctx, item.ID, "approver1", now); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, StatusDone)
	}
	if got.PreviousStatus != StatusNew {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, StatusNew)
	}
	if got.CompletedByUserID != "approver1" {
		t.Errorf("CompletedByUserID = %q, want %q", got.CompletedByUserID, "approver1")
	}
}

func TestCompleteItem_FromInProgressRecordsPreviousStatusInProgress(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.MarkInProgress(ctx, item.ID)

	if err := s.CompleteItem(ctx, item.ID, "approver1", time.Now()); err != nil {
		t.Fatalf("CompleteItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.PreviousStatus != StatusInProgress {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, StatusInProgress)
	}
}

func TestCompleteItem_AlreadyDoneFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", time.Now())
	_ = s.CompleteItem(ctx, item.ID, "approver1", time.Now())

	err := s.CompleteItem(ctx, item.ID, "approver1", time.Now())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestListUndoable_ScopedToGuildAndWindow(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	inGuild, _ := s.CreateItem(ctx, "guild1", "recent", "user1", now)
	_ = s.CompleteItem(ctx, inGuild.ID, "approver1", now.Add(-1*time.Hour))

	otherGuild, _ := s.CreateItem(ctx, "guild2", "other guild", "user1", now)
	_ = s.CompleteItem(ctx, otherGuild.ID, "approver1", now.Add(-1*time.Hour))

	tooOld, _ := s.CreateItem(ctx, "guild1", "old", "user1", now)
	_ = s.CompleteItem(ctx, tooOld.ID, "approver1", now.Add(-30*time.Hour))

	items, err := s.ListUndoable(ctx, "guild1", now)
	if err != nil {
		t.Fatalf("ListUndoable() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ID != inGuild.ID {
		t.Errorf("items[0].ID = %q, want %q", items[0].ID, inGuild.ID)
	}
}

func TestUndoItem_RestoresRecordedPreviousStatus(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
	_ = s.MarkInProgress(ctx, item.ID)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	if err := s.UndoItem(ctx, item.ID, "new-message-id", now); err != nil {
		t.Fatalf("UndoItem() error = %v", err)
	}

	got, _ := s.GetItem(ctx, item.ID)
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
	if got.MessageID != "new-message-id" {
		t.Errorf("MessageID = %q, want %q", got.MessageID, "new-message-id")
	}
	if got.CompletedByUserID != "" {
		t.Errorf("CompletedByUserID = %q, want empty", got.CompletedByUserID)
	}
}

func TestUndoItem_OutsideWindowFails(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now.Add(-25*time.Hour))

	err := s.UndoItem(ctx, item.ID, "new-message-id", now)
	if !errors.Is(err, ErrNotUndoable) {
		t.Fatalf("err = %v, want ErrNotUndoable", err)
	}
}

func TestGuildConfig_DefaultsWhenUnconfigured(t *testing.T) {
	s := newTestService()
	cfg, err := s.GetGuildConfig(context.Background(), "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig() error = %v", err)
	}
	if cfg.InProgressEmote != DefaultInProgressEmote {
		t.Errorf("InProgressEmote = %q, want %q", cfg.InProgressEmote, DefaultInProgressEmote)
	}
	if cfg.DoneEmote != DefaultDoneEmote {
		t.Errorf("DoneEmote = %q, want %q", cfg.DoneEmote, DefaultDoneEmote)
	}
	if cfg.ActionItemsChannelID != "" {
		t.Errorf("ActionItemsChannelID = %q, want empty", cfg.ActionItemsChannelID)
	}
}

func TestSetActionItemsChannel_Persists(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	if err := s.SetActionItemsChannel(ctx, "guild1", "chan1"); err != nil {
		t.Fatalf("SetActionItemsChannel() error = %v", err)
	}

	cfg, _ := s.GetGuildConfig(ctx, "guild1")
	if cfg.ActionItemsChannelID != "chan1" {
		t.Errorf("ActionItemsChannelID = %q, want %q", cfg.ActionItemsChannelID, "chan1")
	}
}

func TestSetEmotes_RejectsEmptyValues(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	err := s.SetEmotes(ctx, "guild1", "", "✅")
	if !errors.Is(err, ErrInvalidEmote) {
		t.Fatalf("err = %v, want ErrInvalidEmote", err)
	}

	err = s.SetEmotes(ctx, "guild1", "🔄", "")
	if !errors.Is(err, ErrInvalidEmote) {
		t.Fatalf("err = %v, want ErrInvalidEmote", err)
	}
}

func TestSetEmotes_ValidValuesPersist(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	if err := s.SetEmotes(ctx, "guild1", "👀", "🎉"); err != nil {
		t.Fatalf("SetEmotes() error = %v", err)
	}

	cfg, _ := s.GetGuildConfig(ctx, "guild1")
	if cfg.InProgressEmote != "👀" || cfg.DoneEmote != "🎉" {
		t.Errorf("emotes = %q/%q, want %q/%q", cfg.InProgressEmote, cfg.DoneEmote, "👀", "🎉")
	}
}

func TestApprovers_AddRemoveList(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	_ = s.AddApprover(ctx, "guild1", "user1")
	_ = s.AddApprover(ctx, "guild1", "user2")

	list, err := s.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	_ = s.RemoveApprover(ctx, "guild1", "user1")
	list, _ = s.ListApprovers(ctx, "guild1")
	if len(list) != 1 || list[0] != "user2" {
		t.Errorf("list = %v, want [user2]", list)
	}
}

func TestIsApprover_MatchesOwnerRoleOrUser(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	_ = s.AddApprover(ctx, "guild1", "user1")
	_ = s.SetApproverRole(ctx, "guild1", "role-a")

	ok, err := s.IsApprover(ctx, "guild1", "user1", nil)
	if err != nil || !ok {
		t.Errorf("IsApprover(user1) = %v, %v, want true, nil", ok, err)
	}

	ok, err = s.IsApprover(ctx, "guild1", "user2", []string{"role-a"})
	if err != nil || !ok {
		t.Errorf("IsApprover(user2 with role-a) = %v, %v, want true, nil", ok, err)
	}

	ok, err = s.IsApprover(ctx, "guild1", "user3", []string{"role-b"})
	if err != nil || ok {
		t.Errorf("IsApprover(user3) = %v, %v, want false, nil", ok, err)
	}
}
```

- [ ] **Step 10: Run tests to verify they fail to compile**

Run: `go test ./internal/actionitems/... -v`
Expected: FAIL — compile errors against the still-old `service.go` (`s.CreateItem` wrong arity, `MarkInProgress` undefined, etc).

- [ ] **Step 11: Rewrite `service.go`**

```go
package actionitems

import (
	"context"
	"errors"
	"time"
)

const (
	undoWindow = 24 * time.Hour
	undoLimit  = 5
)

var (
	ErrNotUndoable       = errors.New("action item is not eligible for undo")
	ErrInvalidTransition = errors.New("action item is not in a state that allows this transition")
	ErrInvalidEmote      = errors.New("emote must not be empty")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateItem(ctx context.Context, guildID, description, createdByUserID string, now time.Time) (ActionItem, error) {
	return s.repo.Create(ctx, ActionItem{
		GuildID:         guildID,
		Description:     description,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		Status:          StatusNew,
	})
}

func (s *Service) AttachMessage(ctx context.Context, id, messageID string) error {
	return s.repo.UpdateMessageID(ctx, id, messageID)
}

func (s *Service) FindPendingByMessage(ctx context.Context, messageID string) (ActionItem, bool, error) {
	item, err := s.repo.FindPendingByMessageID(ctx, messageID)
	if errors.Is(err, ErrNotFound) {
		return ActionItem{}, false, nil
	}
	if err != nil {
		return ActionItem{}, false, err
	}
	return item, true, nil
}

func (s *Service) GetItem(ctx context.Context, id string) (ActionItem, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) MarkInProgress(ctx context.Context, id string) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusNew {
		return ErrInvalidTransition
	}
	return s.repo.SetStatus(ctx, id, StatusInProgress)
}

func (s *Service) MarkNew(ctx context.Context, id string) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusInProgress {
		return ErrInvalidTransition
	}
	return s.repo.SetStatus(ctx, id, StatusNew)
}

func (s *Service) CompleteItem(ctx context.Context, id, completedByUserID string, now time.Time) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status == StatusDone {
		return ErrInvalidTransition
	}
	return s.repo.Complete(ctx, id, completedByUserID, now, item.Status)
}

func (s *Service) ListUndoable(ctx context.Context, guildID string, now time.Time) ([]ActionItem, error) {
	return s.repo.ListCompletedSince(ctx, guildID, now.Add(-undoWindow), undoLimit)
}

func (s *Service) UndoItem(ctx context.Context, id, newMessageID string, now time.Time) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusDone || item.CompletedAt == nil || item.CompletedAt.Before(now.Add(-undoWindow)) {
		return ErrNotUndoable
	}
	restoreStatus := item.PreviousStatus
	if restoreStatus == "" {
		restoreStatus = StatusNew
	}
	return s.repo.Reopen(ctx, id, newMessageID, restoreStatus)
}

func (s *Service) GetGuildConfig(ctx context.Context, guildID string) (GuildConfig, error) {
	return s.repo.GetGuildConfig(ctx, guildID)
}

func (s *Service) SetActionItemsChannel(ctx context.Context, guildID, channelID string) error {
	return s.repo.SetActionItemsChannel(ctx, guildID, channelID)
}

func (s *Service) SetApproverRole(ctx context.Context, guildID, roleID string) error {
	return s.repo.SetApproverRole(ctx, guildID, roleID)
}

func (s *Service) SetEmotes(ctx context.Context, guildID, inProgressEmote, doneEmote string) error {
	if inProgressEmote == "" || doneEmote == "" {
		return ErrInvalidEmote
	}
	return s.repo.SetEmotes(ctx, guildID, inProgressEmote, doneEmote)
}

func (s *Service) SetHelpMessageID(ctx context.Context, guildID, messageID string) error {
	return s.repo.SetHelpMessageID(ctx, guildID, messageID)
}

func (s *Service) AddApprover(ctx context.Context, guildID, userID string) error {
	return s.repo.AddApprover(ctx, guildID, userID)
}

func (s *Service) RemoveApprover(ctx context.Context, guildID, userID string) error {
	return s.repo.RemoveApprover(ctx, guildID, userID)
}

func (s *Service) ListApprovers(ctx context.Context, guildID string) ([]string, error) {
	return s.repo.ListApprovers(ctx, guildID)
}

func (s *Service) IsApprover(ctx context.Context, guildID, userID string, memberRoleIDs []string) (bool, error) {
	cfg, err := s.repo.GetGuildConfig(ctx, guildID)
	if err != nil {
		return false, err
	}
	approvers, err := s.repo.ListApprovers(ctx, guildID)
	if err != nil {
		return false, err
	}
	return isApproverMatch(userID, memberRoleIDs, approvers, cfg.ApproverRoleID), nil
}
```

- [ ] **Step 12: Run all actionitems tests to verify they pass**

Run: `go test ./internal/actionitems/... -v`
Expected: PASS, every test listed in Steps 5 and 9.

- [ ] **Step 13: Commit**

```bash
git add internal/actionitems/
git commit -m "feat: rebuild actionitems domain layer for multi-tenant state machine"
```

---

### Task 3: Postgres migration and repository implementation

**Files:**
- Create: `internal/store/postgres/migrations/0002_multi_tenant.up.sql`
- Create: `internal/store/postgres/migrations/0002_multi_tenant.down.sql`
- Modify: `internal/store/postgres/repository.go`
- Modify: `internal/store/postgres/repository_integration_test.go`

**Interfaces:**
- Consumes: `actionitems.Repository` interface, `actionitems.ActionItem`, `actionitems.GuildConfig`, `actionitems.Status*`, `actionitems.DefaultInProgressEmote/DoneEmote` from Task 2.
- Produces: `postgres.Repository` satisfying the full `actionitems.Repository` interface (compile-time check already exists at the top of `repository.go`: `var _ actionitems.Repository = (*Repository)(nil)`).

- [ ] **Step 1: Write the migration files**

`internal/store/postgres/migrations/0002_multi_tenant.up.sql`:

```sql
ALTER TABLE action_items ADD COLUMN guild_id TEXT NOT NULL;
ALTER TABLE action_items ADD COLUMN previous_status TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_action_items_message_id;
CREATE INDEX idx_action_items_message_id ON action_items (message_id) WHERE status <> 'done';

DROP INDEX IF EXISTS idx_action_items_completed_at;
CREATE INDEX idx_action_items_completed_at ON action_items (completed_at) WHERE status = 'done';

CREATE INDEX idx_action_items_guild_id ON action_items (guild_id);

CREATE TABLE guild_configs (
    guild_id TEXT PRIMARY KEY,
    action_items_channel_id TEXT NOT NULL DEFAULT '',
    approver_role_id TEXT NOT NULL DEFAULT '',
    in_progress_emote TEXT NOT NULL DEFAULT '🔄',
    done_emote TEXT NOT NULL DEFAULT '✅',
    help_message_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE approvers (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (guild_id, user_id)
);
```

`internal/store/postgres/migrations/0002_multi_tenant.down.sql`:

```sql
DROP TABLE IF EXISTS approvers;
DROP TABLE IF EXISTS guild_configs;

DROP INDEX IF EXISTS idx_action_items_guild_id;

DROP INDEX IF EXISTS idx_action_items_completed_at;
CREATE INDEX idx_action_items_completed_at ON action_items (completed_at) WHERE status = 'completed';

DROP INDEX IF EXISTS idx_action_items_message_id;
CREATE INDEX idx_action_items_message_id ON action_items (message_id) WHERE status = 'pending';

ALTER TABLE action_items DROP COLUMN previous_status;
ALTER TABLE action_items DROP COLUMN guild_id;
```

**Note for the executor:** `ADD COLUMN guild_id TEXT NOT NULL` with no default will fail if `action_items` has any existing rows. This deployment only has manual test data, so before applying this migration against the local dev Postgres, truncate the table: `docker compose exec postgres psql -U action_items -d action_items -c "TRUNCATE action_items"` (or just `docker compose down -v && docker compose up -d postgres` to start clean).

- [ ] **Step 2: Truncate the local dev table so the migration can apply**

Run: `docker compose up -d postgres` then `docker compose exec postgres psql -U action_items -d action_items -c "TRUNCATE action_items"`
Expected: `TRUNCATE TABLE`

- [ ] **Step 3: Write the failing integration tests**

Append to `internal/store/postgres/repository_integration_test.go` (keep the existing tests — they'll be updated in Step 5 to match the new signatures, not deleted):

```go
func TestRepository_GuildConfig_DefaultsWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	cfg, err := repo.GetGuildConfig(ctx, "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig: %v", err)
	}
	if cfg.InProgressEmote != actionitems.DefaultInProgressEmote {
		t.Errorf("InProgressEmote = %q, want %q", cfg.InProgressEmote, actionitems.DefaultInProgressEmote)
	}
	if cfg.DoneEmote != actionitems.DefaultDoneEmote {
		t.Errorf("DoneEmote = %q, want %q", cfg.DoneEmote, actionitems.DefaultDoneEmote)
	}
	if cfg.ActionItemsChannelID != "" {
		t.Errorf("ActionItemsChannelID = %q, want empty", cfg.ActionItemsChannelID)
	}
}

func TestRepository_GuildConfig_SetAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	if err := repo.SetActionItemsChannel(ctx, "guild1", "chan1"); err != nil {
		t.Fatalf("SetActionItemsChannel: %v", err)
	}
	if err := repo.SetApproverRole(ctx, "guild1", "role1"); err != nil {
		t.Fatalf("SetApproverRole: %v", err)
	}
	if err := repo.SetEmotes(ctx, "guild1", "👀", "🎉"); err != nil {
		t.Fatalf("SetEmotes: %v", err)
	}
	if err := repo.SetHelpMessageID(ctx, "guild1", "help-msg-1"); err != nil {
		t.Fatalf("SetHelpMessageID: %v", err)
	}

	cfg, err := repo.GetGuildConfig(ctx, "guild1")
	if err != nil {
		t.Fatalf("GetGuildConfig: %v", err)
	}
	if cfg.ActionItemsChannelID != "chan1" {
		t.Errorf("ActionItemsChannelID = %q, want %q", cfg.ActionItemsChannelID, "chan1")
	}
	if cfg.ApproverRoleID != "role1" {
		t.Errorf("ApproverRoleID = %q, want %q", cfg.ApproverRoleID, "role1")
	}
	if cfg.InProgressEmote != "👀" || cfg.DoneEmote != "🎉" {
		t.Errorf("emotes = %q/%q, want 👀/🎉", cfg.InProgressEmote, cfg.DoneEmote)
	}
	if cfg.HelpMessageID != "help-msg-1" {
		t.Errorf("HelpMessageID = %q, want %q", cfg.HelpMessageID, "help-msg-1")
	}
}

func TestRepository_Approvers_AddRemoveList(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	if err := repo.AddApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("AddApprover: %v", err)
	}
	if err := repo.AddApprover(ctx, "guild1", "user2"); err != nil {
		t.Fatalf("AddApprover: %v", err)
	}
	// idempotent re-add
	if err := repo.AddApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("AddApprover (repeat): %v", err)
	}

	list, err := repo.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	if err := repo.RemoveApprover(ctx, "guild1", "user1"); err != nil {
		t.Fatalf("RemoveApprover: %v", err)
	}
	list, err = repo.ListApprovers(ctx, "guild1")
	if err != nil {
		t.Fatalf("ListApprovers: %v", err)
	}
	if len(list) != 1 || list[0] != "user2" {
		t.Errorf("list = %v, want [user2]", list)
	}
}

func TestRepository_StateTransitions_SetStatusAndComplete(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "task", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.SetStatus(ctx, item.ID, actionitems.StatusInProgress); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, _ := repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusInProgress)
	}

	completedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Complete(ctx, item.ID, "approver1", completedAt, actionitems.StatusInProgress); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusDone)
	}
	if got.PreviousStatus != actionitems.StatusInProgress {
		t.Errorf("PreviousStatus = %q, want %q", got.PreviousStatus, actionitems.StatusInProgress)
	}

	if err := repo.Reopen(ctx, item.ID, "new-msg", actionitems.StatusInProgress); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != actionitems.StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, actionitems.StatusInProgress)
	}
	if got.PreviousStatus != "" {
		t.Errorf("PreviousStatus = %q, want empty", got.PreviousStatus)
	}
}

func TestRepository_FindPendingByMessageID_ExcludesDoneOnly(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	item, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "task", CreatedByUserID: "user1", CreatedAt: time.Now(), Status: actionitems.StatusNew,
	})
	_ = repo.UpdateMessageID(ctx, item.ID, "msg1")
	_ = repo.SetStatus(ctx, item.ID, actionitems.StatusInProgress)

	found, err := repo.FindPendingByMessageID(ctx, "msg1")
	if err != nil {
		t.Fatalf("FindPendingByMessageID: %v", err)
	}
	if found.ID != item.ID {
		t.Errorf("ID = %q, want %q", found.ID, item.ID)
	}

	_ = repo.Complete(ctx, item.ID, "approver1", time.Now(), actionitems.StatusInProgress)
	_, err = repo.FindPendingByMessageID(ctx, "msg1")
	if err != actionitems.ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_ListCompletedSince_ScopedToGuild(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	inGuild, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "in guild", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, inGuild.ID, "approver1", now.Add(-1*time.Hour), actionitems.StatusNew)

	otherGuild, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild2", Description: "other guild", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, otherGuild.ID, "approver1", now.Add(-1*time.Hour), actionitems.StatusNew)

	got, err := repo.ListCompletedSince(ctx, "guild1", now.Add(-24*time.Hour), 5)
	if err != nil {
		t.Fatalf("ListCompletedSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != inGuild.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, inGuild.ID)
	}
}
```

- [ ] **Step 4: Update the existing integration tests to match the new signatures**

In `internal/store/postgres/repository_integration_test.go`:

- `TestRepository_CreateAndGet`: add `GuildID: "guild1",` to the `actionitems.ActionItem{...}` literal, and change `Status: actionitems.StatusPending` to `Status: actionitems.StatusNew`.
- `TestRepository_UpdateMessageID`, `TestRepository_FindPendingByMessageID` (the original one — keep both it and the new `TestRepository_FindPendingByMessageID_ExcludesDoneOnly`), `TestRepository_CompleteAndReopen`, `TestRepository_ListCompletedSince`: add `GuildID: "guild1",` to every `actionitems.ActionItem{...}` literal and change `actionitems.StatusPending` to `actionitems.StatusNew`.
- `TestRepository_CompleteAndReopen`: change the `repo.Complete(ctx, item.ID, "approver1", completedAt)` call to `repo.Complete(ctx, item.ID, "approver1", completedAt, actionitems.StatusNew)`, and the `repo.Reopen(ctx, item.ID, "newmsg456")` call to `repo.Reopen(ctx, item.ID, "newmsg456", actionitems.StatusNew)`. Add an assertion that `got.PreviousStatus == ""` after reopen.
- `TestRepository_ListCompletedSince`: change both `repo.Complete(ctx, recent.ID, "approver1", now.Add(-1*time.Hour))` / `repo.Complete(ctx, old.ID, "approver1", now.Add(-30*time.Hour))` calls to pass `actionitems.StatusNew` as a fifth argument, and change the final call from `repo.ListCompletedSince(ctx, now.Add(-24*time.Hour), 5)` to `repo.ListCompletedSince(ctx, "guild1", now.Add(-24*time.Hour), 5)`.

- [ ] **Step 5: Run integration tests to verify they fail to compile**

Run: `docker compose up -d postgres && go test -tags=integration ./internal/store/postgres/... -v`
Expected: FAIL — compile errors, since `repository.go` still has the old method signatures.

- [ ] **Step 6: Rewrite `repository.go`**

```go
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var _ actionitems.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() {
	r.pool.Close()
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// Migrate applies all pending schema migrations to the database at databaseURL.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

const selectColumns = `id, guild_id, description, created_by_user_id, created_at, message_id, status, previous_status, completed_by_user_id, completed_at`

func scanItem(row scanner) (actionitems.ActionItem, error) {
	var item actionitems.ActionItem
	var status, previousStatus string
	err := row.Scan(
		&item.ID,
		&item.GuildID,
		&item.Description,
		&item.CreatedByUserID,
		&item.CreatedAt,
		&item.MessageID,
		&status,
		&previousStatus,
		&item.CompletedByUserID,
		&item.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return actionitems.ActionItem{}, actionitems.ErrNotFound
	}
	if err != nil {
		return actionitems.ActionItem{}, fmt.Errorf("scanning action item: %w", err)
	}
	item.Status = actionitems.Status(status)
	item.PreviousStatus = actionitems.Status(previousStatus)
	return item, nil
}

func (r *Repository) Create(ctx context.Context, item actionitems.ActionItem) (actionitems.ActionItem, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO action_items (guild_id, description, created_by_user_id, created_at, message_id, status, previous_status)
		VALUES ($1, $2, $3, $4, $5, $6, '')
		RETURNING `+selectColumns,
		item.GuildID, item.Description, item.CreatedByUserID, item.CreatedAt, item.MessageID, string(item.Status),
	)
	return scanItem(row)
}

func (r *Repository) Get(ctx context.Context, id string) (actionitems.ActionItem, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM action_items WHERE id = $1`, id)
	return scanItem(row)
}

func (r *Repository) UpdateMessageID(ctx context.Context, id, messageID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE action_items SET message_id = $1 WHERE id = $2`, messageID, id)
	if err != nil {
		return fmt.Errorf("updating message id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}

func (r *Repository) FindPendingByMessageID(ctx context.Context, messageID string) (actionitems.ActionItem, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM action_items WHERE message_id = $1 AND status <> $2 LIMIT 1`,
		messageID, string(actionitems.StatusDone))
	return scanItem(row)
}

func (r *Repository) SetStatus(ctx context.Context, id string, status actionitems.Status) error {
	tag, err := r.pool.Exec(ctx, `UPDATE action_items SET status = $1 WHERE id = $2`, string(status), id)
	if err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}

func (r *Repository) Complete(ctx context.Context, id, completedByUserID string, completedAt time.Time, previousStatus actionitems.Status) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE action_items
		SET status = $1, completed_by_user_id = $2, completed_at = $3, previous_status = $4
		WHERE id = $5`,
		string(actionitems.StatusDone), completedByUserID, completedAt, string(previousStatus), id,
	)
	if err != nil {
		return fmt.Errorf("completing action item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}

func (r *Repository) ListCompletedSince(ctx context.Context, guildID string, since time.Time, limit int) ([]actionitems.ActionItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+selectColumns+` FROM action_items
		WHERE guild_id = $1 AND status = $2 AND completed_at >= $3
		ORDER BY completed_at DESC
		LIMIT $4`,
		guildID, string(actionitems.StatusDone), since, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing completed action items: %w", err)
	}
	defer rows.Close()

	var result []actionitems.ActionItem
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating completed action items: %w", err)
	}
	return result, nil
}

func (r *Repository) Reopen(ctx context.Context, id, newMessageID string, restoreStatus actionitems.Status) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE action_items
		SET status = $1, message_id = $2, completed_by_user_id = '', completed_at = NULL, previous_status = ''
		WHERE id = $3`,
		string(restoreStatus), newMessageID, id,
	)
	if err != nil {
		return fmt.Errorf("reopening action item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}

func (r *Repository) upsertGuildConfig(ctx context.Context, guildID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO guild_configs (guild_id, in_progress_emote, done_emote)
		VALUES ($1, $2, $3)
		ON CONFLICT (guild_id) DO NOTHING`,
		guildID, actionitems.DefaultInProgressEmote, actionitems.DefaultDoneEmote,
	)
	return err
}

func (r *Repository) GetGuildConfig(ctx context.Context, guildID string) (actionitems.GuildConfig, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT guild_id, action_items_channel_id, approver_role_id, in_progress_emote, done_emote, help_message_id
		FROM guild_configs WHERE guild_id = $1`, guildID)
	var cfg actionitems.GuildConfig
	err := row.Scan(&cfg.GuildID, &cfg.ActionItemsChannelID, &cfg.ApproverRoleID, &cfg.InProgressEmote, &cfg.DoneEmote, &cfg.HelpMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return actionitems.GuildConfig{
			GuildID:         guildID,
			InProgressEmote: actionitems.DefaultInProgressEmote,
			DoneEmote:       actionitems.DefaultDoneEmote,
		}, nil
	}
	if err != nil {
		return actionitems.GuildConfig{}, fmt.Errorf("getting guild config: %w", err)
	}
	return cfg, nil
}

func (r *Repository) SetActionItemsChannel(ctx context.Context, guildID, channelID string) error {
	if err := r.upsertGuildConfig(ctx, guildID); err != nil {
		return fmt.Errorf("upserting guild config: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE guild_configs SET action_items_channel_id = $1 WHERE guild_id = $2`, channelID, guildID); err != nil {
		return fmt.Errorf("setting action items channel: %w", err)
	}
	return nil
}

func (r *Repository) SetApproverRole(ctx context.Context, guildID, roleID string) error {
	if err := r.upsertGuildConfig(ctx, guildID); err != nil {
		return fmt.Errorf("upserting guild config: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE guild_configs SET approver_role_id = $1 WHERE guild_id = $2`, roleID, guildID); err != nil {
		return fmt.Errorf("setting approver role: %w", err)
	}
	return nil
}

func (r *Repository) SetEmotes(ctx context.Context, guildID, inProgressEmote, doneEmote string) error {
	if err := r.upsertGuildConfig(ctx, guildID); err != nil {
		return fmt.Errorf("upserting guild config: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE guild_configs SET in_progress_emote = $1, done_emote = $2 WHERE guild_id = $3`, inProgressEmote, doneEmote, guildID); err != nil {
		return fmt.Errorf("setting emotes: %w", err)
	}
	return nil
}

func (r *Repository) SetHelpMessageID(ctx context.Context, guildID, messageID string) error {
	if err := r.upsertGuildConfig(ctx, guildID); err != nil {
		return fmt.Errorf("upserting guild config: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE guild_configs SET help_message_id = $1 WHERE guild_id = $2`, messageID, guildID); err != nil {
		return fmt.Errorf("setting help message id: %w", err)
	}
	return nil
}

func (r *Repository) AddApprover(ctx context.Context, guildID, userID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO approvers (guild_id, user_id) VALUES ($1, $2)
		ON CONFLICT (guild_id, user_id) DO NOTHING`, guildID, userID)
	if err != nil {
		return fmt.Errorf("adding approver: %w", err)
	}
	return nil
}

func (r *Repository) RemoveApprover(ctx context.Context, guildID, userID string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM approvers WHERE guild_id = $1 AND user_id = $2`, guildID, userID); err != nil {
		return fmt.Errorf("removing approver: %w", err)
	}
	return nil
}

func (r *Repository) ListApprovers(ctx context.Context, guildID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM approvers WHERE guild_id = $1 ORDER BY user_id`, guildID)
	if err != nil {
		return nil, fmt.Errorf("listing approvers: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scanning approver: %w", err)
		}
		result = append(result, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating approvers: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 7: Also update `newTestRepository`'s truncation to cover the new tables**

In `internal/store/postgres/repository_integration_test.go`, change the `TRUNCATE` call inside `newTestRepository`:

```go
	if _, err := repo.pool.Exec(context.Background(), "TRUNCATE action_items, guild_configs, approvers"); err != nil {
		t.Fatalf("truncating tables: %v", err)
	}
```

- [ ] **Step 8: Run integration tests to verify they pass**

Run: `docker compose up -d postgres && go test -tags=integration ./internal/store/postgres/... -v`
Expected: PASS, all tests (original ones updated in Step 4 plus the new ones from Step 3).

- [ ] **Step 9: Run the down migration to confirm it's valid, then re-apply up**

Run:
```bash
go run -tags=integration ./cmd/bot 2>&1 | head -1 || true
```
This will fail (no Discord token set) — that's fine, it's not what we're checking. Instead, validate the down migration directly with the `migrate` CLI if installed, or skip if not available; the up migration having applied successfully in Step 8 (via `postgres.Migrate` inside `newTestRepository`) is sufficient confidence for this task. Note in the PR/commit message if the down migration wasn't independently exercised.

- [ ] **Step 10: Commit**

```bash
git add internal/store/postgres/
git commit -m "feat: add multi-tenant postgres schema and repository methods"
```

---

### Task 4: Discord permission helper and bot wiring

**Files:**
- Create: `internal/discord/permissions.go`
- Modify: `internal/discord/bot.go`

**Interfaces:**
- Consumes: `actionitems.Service.IsApprover` from Task 2.
- Produces: `(b *Bot) isOwnerOrApprover(ctx context.Context, guildID string, member *discordgo.Member) (bool, error)`, `(b *Bot) resolveMember(s *discordgo.Session, guildID, userID string, embedded *discordgo.Member) (*discordgo.Member, error)`. `Bot{Session *discordgo.Session; service *actionitems.Service}` (no more `approvers`, `guildID`, `actionItemsChannelID` fields). `discord.New(token string, service *actionitems.Service) (*Bot, error)`.

This is Discord-session glue code (needs a live guild/API lookup), so per the Global Constraints it is not independently unit tested — verified by `go build`/`go vet` and later manual testing.

- [ ] **Step 1: Write `permissions.go`**

```go
package discord

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// isOwnerOrApprover reports whether member is allowed to manage this guild's
// action items configuration or transition/undo items: either the guild
// owner (checked live against the Discord API), or a configured approver.
func (b *Bot) isOwnerOrApprover(ctx context.Context, guildID string, member *discordgo.Member) (bool, error) {
	if member == nil || member.User == nil {
		return false, nil
	}

	guild, err := b.Session.State.Guild(guildID)
	if err != nil {
		guild, err = b.Session.Guild(guildID)
		if err != nil {
			return false, err
		}
	}
	if guild.OwnerID == member.User.ID {
		return true, nil
	}

	return b.service.IsApprover(ctx, guildID, member.User.ID, member.Roles)
}

// resolveMember returns embedded if non-nil (Discord includes it on some
// gateway events), otherwise fetches the member via the REST API.
func (b *Bot) resolveMember(s *discordgo.Session, guildID, userID string, embedded *discordgo.Member) (*discordgo.Member, error) {
	if embedded != nil {
		return embedded, nil
	}
	return s.GuildMember(guildID, userID)
}
```

- [ ] **Step 2: Rewrite `bot.go`**

```go
package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const commandAckEmoji = "✅"

type Bot struct {
	Session *discordgo.Session
	service *actionitems.Service
}

func New(token string, service *actionitems.Service) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuildMembers

	b := &Bot{
		Session: session,
		service: service,
	}
	session.AddHandler(b.handleInteraction)
	session.AddHandler(b.handleReactionAdd)
	session.AddHandler(b.handleReactionRemove)
	return b, nil
}

func (b *Bot) Open() error {
	return b.Session.Open()
}

func (b *Bot) Close() error {
	return b.Session.Close()
}

// RegisterCommands registers the bot's slash commands globally, so they
// work in any guild the bot is invited to without a per-guild step.
func (b *Bot) RegisterCommands() error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "action-item",
			Description: "Create a new action item",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "text",
					Description: "The action item description",
					Required:    true,
				},
			},
		},
		{
			Name:        "undo",
			Description: "Undo a recently completed action item",
		},
		{
			Name:        "approver",
			Description: "Manage who can transition and undo action items",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Add an approver",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to add", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove an approver",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to remove", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List configured approvers",
				},
			},
		},
		{
			Name:        "config",
			Description: "Open the action items configuration panel",
		},
	}

	for _, cmd := range commands {
		if _, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, "", cmd); err != nil {
			return fmt.Errorf("registering command %s: %w", cmd.Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Build to verify (expected to fail — other files still reference the old `Bot` fields/handlers)**

Run: `go build ./...`
Expected: FAIL — `commands.go`, `reactions.go`, `helpers.go`, `cmd/bot/main.go` reference removed fields/functions (`b.actionItemsChannelID`, `b.isApprover`, `doneEmoji`, old `discord.New` signature, undefined `handleReactionRemove`/config handlers). This is expected; subsequent tasks fix each.

- [ ] **Step 4: Commit**

```bash
git add internal/discord/permissions.go internal/discord/bot.go
git commit -m "feat: multi-tenant bot wiring, global command registration, owner-or-approver check"
```

(The build stays red until Task 8 completes — that's expected for this multi-file rewrite; each task's diff is still independently reviewable.)

---

### Task 5: Discord pure helpers

**Files:**
- Modify: `internal/discord/helpers.go`
- Modify: `internal/discord/helpers_test.go`

**Interfaces:**
- Consumes: `actionitems.Status*`, `actionitems.GuildConfig` from Task 2.
- Produces (consumed by Tasks 6-9): `prefixForStatus(status actionitems.Status) string`, `statusForEmote(cfg actionitems.GuildConfig, emojiName string) (actionitems.Status, bool)`, `subOptionUserID(options []*discordgo.ApplicationCommandInteractionDataOption) string`, `approverListText(approvers []string) string`, `modalEmoteValues(components []discordgo.MessageComponent) (inProgress, done string)`. `actionItemText` and `undoSelectOptions` and `respondEphemeral` are unchanged from today.

- [ ] **Step 1: Read the existing `helpers_test.go` to preserve its existing tests**

Run: `cat internal/discord/helpers_test.go`

The existing tests for `actionItemText`, `undoSelectOptions`, and `respondEphemeral` stay as-is — only new tests are appended in this task, and `helpers.go` keeps those three functions unchanged.

- [ ] **Step 2: Write the failing tests for the new helpers**

Append to `internal/discord/helpers_test.go`:

```go
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
```

Add `"github.com/wwsean08/action-items-bot/internal/actionitems"` to the imports of `helpers_test.go` if not already present (it will already be imported for `undoSelectOptions` tests).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/discord/... -run 'TestPrefixForStatus|TestStatusForEmote|TestSubOptionUserID|TestApproverListText|TestModalEmoteValues' -v`
Expected: FAIL — undefined functions/constants (`prefixForStatus`, `statusForEmote`, `subOptionUserID`, `approverListText`, `modalEmoteValues`, `configInProgressEmoteInputID`, `configDoneEmoteInputID`).

- [ ] **Step 4: Add the new helpers to `helpers.go`**

Append to `internal/discord/helpers.go` (keep the existing `actionItemText`, `undoSelectOptions`, `respondEphemeral` functions and imports unchanged, add `"strings"` to imports):

```go
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
```

Note: `configInProgressEmoteInputID` and `configDoneEmoteInputID` are defined in Task 7 (`config_panel.go`), same package — this file will not compile in isolation until Task 7 lands, consistent with the rest of this plan's multi-file rewrite.

- [ ] **Step 5: Run tests to verify they still fail to compile (expected, constants not yet defined)**

Run: `go build ./internal/discord/...`
Expected: FAIL — `configInProgressEmoteInputID` / `configDoneEmoteInputID` undefined. This is expected per Step 4's note; do not add placeholder consts here — Task 7 defines them for real.

- [ ] **Step 6: Commit**

```bash
git add internal/discord/helpers.go internal/discord/helpers_test.go
git commit -m "feat: add prefix/emote/modal pure helpers for discord layer"
```

---

### Task 6: `/action-item` and `/undo` command updates

**Files:**
- Modify: `internal/discord/commands.go`

**Interfaces:**
- Consumes: `b.isOwnerOrApprover` (Task 4), `prefixForStatus` (Task 5), `service.GetGuildConfig/CreateItem/AttachMessage/ListUndoable/GetItem/UndoItem` (Task 2).
- Produces: `handleActionItemCommand`, `handleUndoCommand`, `handleUndoSelect` — unchanged names, new bodies. `handleInteraction`'s dispatch table grows to route `approver`/`config` commands and the new component/modal custom IDs (finished across this task and Tasks 7-8).

Glue code — no direct unit tests, per Global Constraints.

- [ ] **Step 1: Rewrite `commands.go`**

```go
package discord

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

const undoSelectCustomID = "undo_select"

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch i.ApplicationCommandData().Name {
		case "action-item":
			b.handleActionItemCommand(s, i)
		case "undo":
			b.handleUndoCommand(s, i)
		case "approver":
			b.handleApproverCommand(s, i)
		case "config":
			b.handleConfigCommand(s, i)
		}
	case discordgo.InteractionMessageComponent:
		switch i.MessageComponentData().CustomID {
		case undoSelectCustomID:
			b.handleUndoSelect(s, i)
		case configChannelSelectCustomID:
			b.handleConfigChannelSelect(s, i)
		case configRoleSelectCustomID:
			b.handleConfigRoleSelect(s, i)
		case configEditEmotesButtonCustomID:
			b.handleConfigEditEmotesButton(s, i)
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == configEmotesModalCustomID {
			b.handleConfigEmotesModalSubmit(s, i)
		}
	}
}

func (b *Bot) handleActionItemCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	guildID := i.GuildID
	text := actionItemText(i.ApplicationCommandData().Options)

	cfg, err := b.service.GetGuildConfig(ctx, guildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to create action item.")
		return
	}
	if cfg.ActionItemsChannelID == "" {
		_ = respondEphemeral(s, i, "This server hasn't configured an action items channel yet. Ask an approver to run /config.")
		return
	}

	item, err := b.service.CreateItem(ctx, guildID, text, i.Member.User.ID, time.Now())
	if err != nil {
		log.Printf("create action item: %v", err)
		_ = respondEphemeral(s, i, "Failed to create action item.")
		return
	}

	posted := prefixForStatus(actionitems.StatusNew) + text
	msg, err := s.ChannelMessageSend(cfg.ActionItemsChannelID, posted)
	if err != nil {
		log.Printf("posting action item message: %v", err)
		_ = respondEphemeral(s, i, "Created, but failed to post to the action items channel.")
		return
	}

	if err := b.service.AttachMessage(ctx, item.ID, msg.ID); err != nil {
		log.Printf("attaching message id: %v", err)
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Action item created: %s", text),
		},
	})
	if err != nil {
		log.Printf("responding to interaction: %v", err)
		return
	}

	reply, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Printf("fetching interaction response: %v", err)
		return
	}
	if err := s.MessageReactionAdd(reply.ChannelID, reply.ID, commandAckEmoji); err != nil {
		log.Printf("reacting to confirmation: %v", err)
	}
}

func (b *Bot) handleUndoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	items, err := b.service.ListUndoable(ctx, i.GuildID, time.Now())
	if err != nil {
		log.Printf("list undoable: %v", err)
		_ = respondEphemeral(s, i, "Failed to look up recent completions.")
		return
	}
	if len(items) == 0 {
		_ = respondEphemeral(s, i, "Nothing to undo.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "Select an action item to restore:",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    undoSelectCustomID,
							Placeholder: "Choose an item to undo",
							Options:     undoSelectOptions(items),
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("responding with undo options: %v", err)
	}
}

func (b *Bot) handleUndoSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to undo action items.")
		return
	}

	values := i.MessageComponentData().Values
	if len(values) == 0 {
		_ = respondEphemeral(s, i, "No item selected.")
		return
	}
	itemID := values[0]

	item, err := b.service.GetItem(ctx, itemID)
	if err != nil {
		log.Printf("get item for undo: %v", err)
		_ = respondEphemeral(s, i, "That action item could no longer be found.")
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, i.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		_ = respondEphemeral(s, i, "Failed to restore that action item.")
		return
	}

	restoreStatus := item.PreviousStatus
	if restoreStatus == "" {
		restoreStatus = actionitems.StatusNew
	}
	posted := prefixForStatus(restoreStatus) + item.Description
	msg, err := s.ChannelMessageSend(cfg.ActionItemsChannelID, posted)
	if err != nil {
		log.Printf("reposting action item: %v", err)
		_ = respondEphemeral(s, i, "Failed to repost the action item.")
		return
	}

	if err := b.service.UndoItem(ctx, itemID, msg.ID, time.Now()); err != nil {
		log.Printf("undo item: %v", err)
		_ = respondEphemeral(s, i, "Failed to undo that action item.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    fmt.Sprintf("Restored: %s", item.Description),
			Components: []discordgo.MessageComponent{},
		},
	})
	if err != nil {
		log.Printf("confirming undo: %v", err)
	}
}
```

- [ ] **Step 2: Build to verify (expected to still fail — `/approver` and `/config` handlers and their custom ID constants don't exist yet)**

Run: `go build ./...`
Expected: FAIL — `handleApproverCommand`, `handleConfigCommand`, `handleConfigChannelSelect`, `handleConfigRoleSelect`, `handleConfigEditEmotesButton`, `handleConfigEmotesModalSubmit`, `configChannelSelectCustomID`, `configRoleSelectCustomID`, `configEditEmotesButtonCustomID`, `configEmotesModalCustomID` undefined. `reactions.go` still references the old single-state API too. All resolved by Tasks 7-9.

- [ ] **Step 3: Commit**

```bash
git add internal/discord/commands.go
git commit -m "feat: scope /action-item and /undo to per-guild config and owner-or-approver"
```

---

### Task 7: `/config` interactive panel

**Files:**
- Create: `internal/discord/config_panel.go`
- Create: `internal/discord/config_panel_test.go`

**Interfaces:**
- Consumes: `b.isOwnerOrApprover` (Task 4), `modalEmoteValues` (Task 5), `service.GetGuildConfig/SetActionItemsChannel/SetApproverRole/SetEmotes` (Task 2), `b.syncHelpMessage` (Task 9 — the calls are added now but `syncHelpMessage` doesn't exist until Task 9, so the package won't build until then; this is consistent with this plan's multi-file rewrite approach).
- Produces: `configChannelSelectCustomID`, `configRoleSelectCustomID`, `configEditEmotesButtonCustomID`, `configEmotesModalCustomID`, `configInProgressEmoteInputID`, `configDoneEmoteInputID` (consumed by Task 5's `modalEmoteValues` and Task 6's dispatch). `configPanelContent(cfg actionitems.GuildConfig) string` and `configPanelComponents(cfg actionitems.GuildConfig) []discordgo.MessageComponent` — pure, unit tested. Handlers `handleConfigCommand`, `handleConfigChannelSelect`, `handleConfigRoleSelect`, `handleConfigEditEmotesButton`, `handleConfigEmotesModalSubmit`, `updateConfigPanel` — glue, untested.

- [ ] **Step 1: Write the failing tests for the pure panel-rendering functions**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/discord/... -run TestConfigPanel -v`
Expected: FAIL — `configPanelContent`, `configPanelComponents`, and the custom ID constants are undefined.

- [ ] **Step 3: Write `config_panel.go`**

```go
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
```

- [ ] **Step 4: Run the panel tests to verify they pass (package as a whole still won't build — `syncHelpMessage` is Task 9)**

Run: `go test ./internal/discord/... -run TestConfigPanel -v`
Expected: FAIL to build the whole package (`b.syncHelpMessage` undefined) — but confirm via `go vet ./internal/discord/config_panel.go internal/discord/config_panel_test.go` style isolated read that the test logic itself is correct; full green comes after Task 9. Note this explicitly rather than skipping verification.

- [ ] **Step 5: Commit**

```bash
git add internal/discord/config_panel.go internal/discord/config_panel_test.go
git commit -m "feat: add /config interactive panel (channel/role selects, emote modal)"
```

---

### Task 8: `/approver` command handlers

**Files:**
- Modify: `internal/discord/commands.go`

**Interfaces:**
- Consumes: `b.isOwnerOrApprover` (Task 4), `subOptionUserID`/`approverListText` (Task 5), `service.AddApprover/RemoveApprover/ListApprovers` (Task 2), `b.syncHelpMessage` (Task 9).
- Produces: `handleApproverCommand`.

- [ ] **Step 1: Append `handleApproverCommand` to `commands.go`**

```go
func (b *Bot) handleApproverCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	allowed, err := b.isOwnerOrApprover(ctx, i.GuildID, i.Member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		_ = respondEphemeral(s, i, "Failed to check permissions.")
		return
	}
	if !allowed {
		_ = respondEphemeral(s, i, "You are not authorized to manage approvers.")
		return
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		_ = respondEphemeral(s, i, "Missing subcommand.")
		return
	}
	sub := opts[0]

	switch sub.Name {
	case "add":
		userID := subOptionUserID(sub.Options)
		if userID == "" {
			_ = respondEphemeral(s, i, "No user specified.")
			return
		}
		if err := b.service.AddApprover(ctx, i.GuildID, userID); err != nil {
			log.Printf("add approver: %v", err)
			_ = respondEphemeral(s, i, "Failed to add approver.")
			return
		}
		if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
			log.Printf("syncing help message: %v", err)
		}
		_ = respondEphemeral(s, i, fmt.Sprintf("Added <@%s> as an approver.", userID))
	case "remove":
		userID := subOptionUserID(sub.Options)
		if userID == "" {
			_ = respondEphemeral(s, i, "No user specified.")
			return
		}
		if err := b.service.RemoveApprover(ctx, i.GuildID, userID); err != nil {
			log.Printf("remove approver: %v", err)
			_ = respondEphemeral(s, i, "Failed to remove approver.")
			return
		}
		if err := b.syncHelpMessage(ctx, i.GuildID); err != nil {
			log.Printf("syncing help message: %v", err)
		}
		_ = respondEphemeral(s, i, fmt.Sprintf("Removed <@%s> as an approver.", userID))
	case "list":
		approvers, err := b.service.ListApprovers(ctx, i.GuildID)
		if err != nil {
			log.Printf("list approvers: %v", err)
			_ = respondEphemeral(s, i, "Failed to list approvers.")
			return
		}
		_ = respondEphemeral(s, i, approverListText(approvers))
	}
}
```

- [ ] **Step 2: Build to verify (expected still red — `reactions.go` and `syncHelpMessage` remain)**

Run: `go build ./...`
Expected: FAIL — `handleReactionRemove`, `b.syncHelpMessage` undefined. Resolved by Tasks 9-10.

- [ ] **Step 3: Commit**

```bash
git add internal/discord/commands.go
git commit -m "feat: add /approver add/remove/list command handlers"
```

---

### Task 9: Pinned help message

**Files:**
- Create: `internal/discord/help_message.go`
- Create: `internal/discord/help_message_test.go`

**Interfaces:**
- Consumes: `service.GetGuildConfig/ListApprovers/SetHelpMessageID` (Task 2).
- Produces: `helpMessageBody(cfg actionitems.GuildConfig, ownerID string, approvers []string) string` (pure, unit tested), `(b *Bot) syncHelpMessage(ctx context.Context, guildID string) error` (glue, untested — satisfies the calls already written into `config_panel.go` and `commands.go`).

- [ ] **Step 1: Write the failing test for the pure body builder**

```go
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

	for _, want := range []string{"🔄", "✅", "<@owner1>", "<@&role1>", "<@user1>", "<@user2>", "/action-item", "/undo", "/config", "/approver"} {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discord/... -run TestHelpMessageBody -v`
Expected: FAIL — `helpMessageBody` undefined.

- [ ] **Step 3: Write `help_message.go`**

```go
package discord

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func helpMessageBody(cfg actionitems.GuildConfig, ownerID string, approvers []string) string {
	var b strings.Builder
	b.WriteString("**📋 Action Items — How This Works**\n\n")
	b.WriteString("**Creating**: use `/action-item text:\"...\"` to post a new item here.\n\n")
	b.WriteString("**Transitions**:\n")
	fmt.Fprintf(&b, "- React with %s to mark an item **in progress**. Remove the reaction to move it back to new.\n", cfg.InProgressEmote)
	fmt.Fprintf(&b, "- React with %s to mark an item **done**. This removes the message; use `/undo` to bring it back.\n\n", cfg.DoneEmote)

	who := []string{fmt.Sprintf("<@%s> (server owner)", ownerID)}
	if cfg.ApproverRoleID != "" {
		who = append(who, fmt.Sprintf("<@&%s>", cfg.ApproverRoleID))
	}
	for _, id := range approvers {
		who = append(who, fmt.Sprintf("<@%s>", id))
	}
	b.WriteString("**Who can do this**: ")
	b.WriteString(strings.Join(who, ", "))
	b.WriteString("\n\n")

	b.WriteString("**Undo**: if an item was completed by mistake, run `/undo` within 24 hours (last 5 completions) to restore it.\n\n")
	b.WriteString("**Configuration**: run `/config` to change the channel, emotes, or approver role. Manage individual approvers with `/approver add`, `/approver remove`, and `/approver list`.")

	return b.String()
}

// syncHelpMessage keeps one pinned explainer message per guild's action
// items channel up to date, editing it in place rather than reposting.
func (b *Bot) syncHelpMessage(ctx context.Context, guildID string) error {
	cfg, err := b.service.GetGuildConfig(ctx, guildID)
	if err != nil {
		return fmt.Errorf("get guild config: %w", err)
	}
	if cfg.ActionItemsChannelID == "" {
		return nil
	}

	guild, err := b.Session.State.Guild(guildID)
	if err != nil {
		guild, err = b.Session.Guild(guildID)
		if err != nil {
			return fmt.Errorf("get guild: %w", err)
		}
	}

	approvers, err := b.service.ListApprovers(ctx, guildID)
	if err != nil {
		return fmt.Errorf("list approvers: %w", err)
	}

	content := helpMessageBody(cfg, guild.OwnerID, approvers)

	if cfg.HelpMessageID != "" {
		if _, err := b.Session.ChannelMessageEdit(cfg.ActionItemsChannelID, cfg.HelpMessageID, content); err == nil {
			return nil
		}
		log.Printf("editing help message failed for guild %s, reposting", guildID)
	}

	msg, err := b.Session.ChannelMessageSend(cfg.ActionItemsChannelID, content)
	if err != nil {
		return fmt.Errorf("posting help message: %w", err)
	}
	if err := b.Session.ChannelMessagePin(cfg.ActionItemsChannelID, msg.ID); err != nil {
		log.Printf("pinning help message: %v", err)
	}
	if err := b.service.SetHelpMessageID(ctx, guildID, msg.ID); err != nil {
		return fmt.Errorf("saving help message id: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/discord/... -run TestHelpMessageBody -v`
Expected: PASS, both cases.

- [ ] **Step 5: Commit**

```bash
git add internal/discord/help_message.go internal/discord/help_message_test.go
git commit -m "feat: add self-updating pinned help message"
```

---

### Task 10: Reactions rewrite for the three-state machine

**Files:**
- Modify: `internal/discord/reactions.go`

**Interfaces:**
- Consumes: `b.resolveMember`/`b.isOwnerOrApprover` (Task 4), `statusForEmote`/`prefixForStatus` (Task 5), `service.FindPendingByMessage/GetGuildConfig/MarkInProgress/MarkNew/CompleteItem` (Task 2).
- Produces: rewritten `handleReactionAdd`, new `handleReactionRemove` (both already wired into `session.AddHandler` in Task 4's `bot.go`).

Glue code — no direct unit tests, per Global Constraints.

- [ ] **Step 1: Rewrite `reactions.go`**

```go
package discord

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)

func (b *Bot) handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return
	}

	ctx := context.Background()
	item, found, err := b.service.FindPendingByMessage(ctx, r.MessageID)
	if err != nil {
		log.Printf("find pending by message: %v", err)
		return
	}
	if !found {
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, r.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		return
	}

	target, recognized := statusForEmote(cfg, r.Emoji.Name)
	if !recognized {
		return
	}

	member, err := b.resolveMember(s, r.GuildID, r.UserID, r.Member)
	if err != nil {
		log.Printf("fetching guild member: %v", err)
		return
	}

	allowed, err := b.isOwnerOrApprover(ctx, r.GuildID, member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		return
	}
	if !allowed {
		return
	}

	switch target {
	case actionitems.StatusInProgress:
		if item.Status != actionitems.StatusNew {
			return
		}
		if err := b.service.MarkInProgress(ctx, item.ID); err != nil {
			log.Printf("marking in progress: %v", err)
			return
		}
		if _, err := s.ChannelMessageEdit(r.ChannelID, r.MessageID, prefixForStatus(actionitems.StatusInProgress)+item.Description); err != nil {
			log.Printf("editing message for in-progress: %v", err)
		}
	case actionitems.StatusDone:
		if err := b.service.CompleteItem(ctx, item.ID, r.UserID, time.Now()); err != nil {
			log.Printf("completing action item: %v", err)
			return
		}
		if err := s.ChannelMessageDelete(r.ChannelID, r.MessageID); err != nil {
			log.Printf("deleting completed action item message: %v", err)
		}
	}
}

func (b *Bot) handleReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return
	}

	ctx := context.Background()
	item, found, err := b.service.FindPendingByMessage(ctx, r.MessageID)
	if err != nil {
		log.Printf("find pending by message: %v", err)
		return
	}
	if !found || item.Status != actionitems.StatusInProgress {
		return
	}

	cfg, err := b.service.GetGuildConfig(ctx, r.GuildID)
	if err != nil {
		log.Printf("get guild config: %v", err)
		return
	}
	if r.Emoji.Name != cfg.InProgressEmote {
		return
	}

	member, err := b.resolveMember(s, r.GuildID, r.UserID, nil)
	if err != nil {
		log.Printf("fetching guild member: %v", err)
		return
	}

	allowed, err := b.isOwnerOrApprover(ctx, r.GuildID, member)
	if err != nil {
		log.Printf("checking approver: %v", err)
		return
	}
	if !allowed {
		return
	}

	if err := b.service.MarkNew(ctx, item.ID); err != nil {
		log.Printf("marking new: %v", err)
		return
	}
	if _, err := s.ChannelMessageEdit(r.ChannelID, r.MessageID, prefixForStatus(actionitems.StatusNew)+item.Description); err != nil {
		log.Printf("editing message for new: %v", err)
	}
}
```

- [ ] **Step 2: Build to verify (expected to fail — `cmd/bot/main.go` still calls the old `discord.New` signature and builds an `ApproverChecker`)**

Run: `go build ./...`
Expected: FAIL — only `cmd/bot/main.go` errors remain (`too many arguments in call to discord.New`, `undefined: actionitems.ApproverChecker`). Resolved in Task 11.

- [ ] **Step 3: Commit**

```bash
git add internal/discord/reactions.go
git commit -m "feat: rewrite reaction handlers for new/in-progress/done state machine"
```

---

### Task 11: Wire up `cmd/bot/main.go` and update deployment files

**Files:**
- Modify: `cmd/bot/main.go`
- Modify: `docker-compose.yml`
- Modify: `.env.example`

**Interfaces:**
- Consumes: `discord.New(token string, service *actionitems.Service) (*Bot, error)` (Task 4). No more `actionitems.ApproverChecker`.

- [ ] **Step 1: Update `cmd/bot/main.go`**

Remove the `approvers := actionitems.ApproverChecker{...}` line and change the `discord.New` call:

```go
	service := actionitems.NewService(repo)

	bot, err := discord.New(cfg.DiscordToken, service)
	if err != nil {
		log.Fatalf("creating bot: %v", err)
	}
```

The rest of `main.go` (config load, migrate, repo connect, bot open/register, health server, signal handling) is unchanged.

- [ ] **Step 2: Update `docker-compose.yml`'s `bot` service environment block**

```yaml
  bot:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DISCORD_TOKEN: ${DISCORD_TOKEN}
      DATABASE_URL: postgres://action_items:action_items@postgres:5432/action_items?sslmode=disable
      HEALTH_PORT: 8080
    ports:
      - "8080:8080"
```

- [ ] **Step 3: Update `.env.example`**

```
DISCORD_TOKEN=
```

- [ ] **Step 4: Build the whole module to verify everything compiles**

Run: `go build ./...`
Expected: PASS, no errors.

- [ ] **Step 5: Run `go vet`**

Run: `go vet ./...`
Expected: PASS, no issues.

- [ ] **Step 6: Commit**

```bash
git add cmd/bot/main.go docker-compose.yml .env.example
git commit -m "feat: wire up multi-tenant bot in main.go and simplify deployment config"
```

---

### Task 12: Full verification pass

**Files:** none (verification only).

- [ ] **Step 1: Run the full unit test suite**

Run: `go test ./... -v`
Expected: PASS across `internal/config`, `internal/actionitems`, `internal/discord`, `internal/health` (unaffected by this plan, should still pass unchanged).

- [ ] **Step 2: Reset the local Postgres to a clean state and run integration tests**

Run:
```bash
docker compose down -v
docker compose up -d postgres
go test -tags=integration ./internal/store/postgres/... -v
```
Expected: PASS, all tests including the new guild-config/approver/state-transition ones from Task 3.

- [ ] **Step 3: Build and smoke-test the bot container**

Run:
```bash
docker compose build bot
docker compose up bot
```
Expected: fails fast with `loading config: DISCORD_TOKEN is required` (no real `.env` present) — same shape of check as when the compose bot service was first wired up, confirming the simplified env var set and dependency ordering still work.

Run: `docker compose rm -f bot`

- [ ] **Step 4: Note the manual, credential-requiring verification still outstanding**

These require a real Discord bot token and two test guilds and cannot be automated here — record them for the user to run:

- In a fresh guild, confirm `/action-item` is rejected with a helpful message until a channel is configured via `/config`.
- As the guild owner (no prior approvers), run `/config` — confirm the panel opens, pick a channel via the select menu and confirm it saves and the panel updates, set a role via the role select, and use "Edit Emotes" to confirm the modal pre-fills current values and saves both on submit. Run `/approver add` as the owner.
- Confirm a pinned message appears in the configured channel listing the current emotes, who's allowed to use them, and undo/action-item usage. Change an emote and re-add an approver — confirm the same pinned message updates in place rather than a new one being posted.
- Create an item, react with the in-progress emote → confirm prefix updates to `[IN PROGRESS]`. Remove the reaction → confirm it reverts to `[NEW]`.
- React with the done emote from both `new` and `in_progress` starting states → confirm the message is deleted both times.
- Run `/undo` → confirm it restores to the correct prior state (test both a new→done and a new→in_progress→done case).
- Confirm a second guild has fully independent config/approvers/items from the first.
- Since commands are now registered globally, note that first-time global command propagation can take up to an hour in Discord — if a new command doesn't appear immediately after `RegisterCommands` runs, that's expected and not a bug.

- [ ] **Step 5: Report status to the user**

Summarize: all automated verification (unit tests, integration tests, container build/smoke-test) passing; list the manual checks from Step 4 as the remaining outstanding verification requiring real Discord credentials.

---

## Self-Review Notes

- **Spec coverage**: every decision in the spec (`docs/superpowers/specs/2026-08-24-multi-tenant-state-machine-design.md`) maps to a task — multi-tenancy (Tasks 2-4, 11), three-state machine (Tasks 2, 3, 10), configurable emotes with defaults (Tasks 2, 3, 7), `/approver`/`/config` commands and owner-or-approver permission model (Tasks 4, 6-9), interactive panel with modal (Task 7), pinned self-updating help message (Task 9), state-prefixed messages (Tasks 5, 6, 10), env var shrink (Tasks 1, 11).
- **Type consistency verified**: `Repository.Complete`/`Reopen`/`ListCompletedSince` signatures match exactly across the interface (Task 2), the fake (Task 2), and the postgres implementation (Task 3). `configInProgressEmoteInputID`/`configDoneEmoteInputID` are defined once in Task 7 and consumed by Task 5's `modalEmoteValues` and Task 7's own modal-building code — no duplicate/conflicting definitions.
- **discordgo API details verified against the installed `v0.29.0` source** (not assumed): `SelectMenuType` constants (`ChannelSelectMenu`, `RoleSelectMenu`), `SelectMenuDefaultValue`/`SelectMenuDefaultValueType`, `ButtonStyle` constants, `TextInput` struct fields, `ModalSubmitInteractionData.Components` decoding into `*discordgo.ActionsRow`/`*discordgo.TextInput` (pointer types, confirmed via `unmarshalableMessageComponent.UnmarshalJSON` in `components.go`), `ChannelTypeGuildText`, `InteractionResponseModal`, `State.Guild`/`Session.Guild`, `ChannelMessageEdit`/`ChannelMessagePin` signatures, `ApplicationCommandOptionUser`/`ApplicationCommandOptionSubCommand`.
- **Known soft spot flagged, not hidden**: Task 3's Step 9 notes the down-migration isn't independently exercised by an automated test in this plan (only implicitly valid by being straightforward inverse SQL) — acceptable given the existing codebase's migration testing convention (the original `0001_init` migration also wasn't down-tested), but worth a manual `migrate down` check if the executor wants extra confidence.
- **No placeholders**: every step has real code, real commands, and real expected output — nothing deferred to "similar to Task N" or "add appropriate handling."
