# Discord Action Items Bot — `/search` Completed-History Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/search query:"..."` slash command that lets anyone a guild's Discord Integrations settings permit search that guild's *completed* (`done`) action item history — case-insensitively, by substring, capped at 10 most-recently-completed results, answered ephemerally.

**Architecture:** Follows the existing three-layer split unchanged — a new `SearchCompleted` method on the `actionitems.Repository` interface, mirrored by the in-memory `fakeRepository` (tests) and the `postgres.Repository` (parameterized `ILIKE ... ESCAPE '\'` query); a thin validating `Service.SearchCompleted` wrapper; and a discord-layer command (registration + dispatch + handler) whose only non-glue logic lives in pure, TDD-tested helpers in `internal/discord/helpers.go`.

**Tech Stack:** Go 1.26.7, discordgo v0.29.0, pgx/v5, golang-migrate/v4, Postgres 16.

**Approved design decisions (settled with the user — do not re-litigate):**
- One required string option, `query`.
- **No bot-enforced permission check** — unlike `/undo`, `/approver`, `/config`, the handler must **not** call `isOwnerOrApprover`. Access is whatever the guild configures in Discord's Integrations settings, same model as `/action-item`.
- Scope: `done` items only, scoped to `i.GuildID`, full history (no time window).
- Matching: case-insensitive substring against the description.
- Cap: 10 results, most-recently-completed first.
- Response: always ephemeral; a plain "no results" ephemeral message when nothing matches.
- SQL safety: the query text is a **bound parameter** (`$3`), never concatenated; LIKE metacharacters (`%`, `_`, `\`) are escaped with an explicit `ESCAPE` clause so they are matched literally.

---

## Global Constraints

- **Never interpolate user input into SQL.** Every value goes through a pgx bind parameter (`$1`, `$2`, …). No `fmt.Sprintf` into a query string, ever — this matches every existing query in `internal/store/postgres/repository.go`.
- **Wildcard escaping is a correctness measure, not the injection defense.** Parameterization is what prevents injection; escaping `%`/`_`/`\` exists so a user searching for a literal `50%` doesn't get wildcard behavior. Both must be present, and the `ESCAPE '\'` clause in the SQL must match the escape character used by the Go helper.
- **No permission check in `/search`'s handler.** Do not import or call `isOwnerOrApprover` there. Do not touch `i.Member`.
- Every registered command carries `Contexts: &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild}` — the new one included. Because of this, `i.GuildID` is always populated in the handler.
- New repository methods keep the compile-time check `var _ actionitems.Repository = (*Repository)(nil)` at the top of `internal/store/postgres/repository.go` passing — adding a method to the interface means the fake **and** postgres implementations must both gain it before the tree builds.
- Result limit lives in exactly one place: `searchLimit = 10` in `internal/actionitems/service.go`, alongside `undoWindow`/`undoLimit`. The repository takes `limit int` as a parameter (same shape as `ListCompletedSince`), it does not hardcode a cap.
- Errors are wrapped with the codebase convention `fmt.Errorf("...: %w", err)` in the postgres layer; sentinel errors live in `internal/actionitems/service.go`'s `var (...)` block next to `ErrNotUndoable`/`ErrInvalidEmote`.
- Testing philosophy, unchanged: service-layer logic and **pure** discord helpers get real TDD unit tests; anything calling `*discordgo.Session` directly stays thin glue verified only by `go build`/`go vet` and manual checks. `fakeRepository` is written directly (not TDD'd) and has no tests of its own.
- **No new migration and no new index.** `ILIKE '%…%'` cannot use a btree index anyway; the existing `idx_action_items_guild_id` bounds the scan and per-guild item volumes are small. If this ever becomes slow, the fix is a `pg_trgm` GIN index — explicitly out of scope here. Do not invent a migration.
- Discord's 2000-character message limit is respected by truncating each result description to 100 characters (the same rule `undoSelectOptions` already applies), keeping a full 10-result response comfortably under ~1400 characters.
- Ephemeral responses reuse the existing `respondEphemeral` helper. It intentionally does **not** set `AllowedMentions` — the mention-safety fix in commit `e56440f` applied only to channel sends (`ChannelMessageSendComplex`), because ephemeral interaction responses are visible only to the invoker and do not notify mentioned users. Do not "fix" `respondEphemeral`.

---

### Task 1: Domain, fake, service, and Postgres search

**Files:**
- Modify: `internal/actionitems/repository.go`
- Modify: `internal/actionitems/fake_repository_test.go`
- Modify: `internal/actionitems/service.go`
- Modify: `internal/actionitems/service_test.go`
- Create: `internal/store/postgres/repository_test.go`
- Modify: `internal/store/postgres/repository.go`
- Modify: `internal/store/postgres/repository_integration_test.go`

**Interfaces:**
- Produces: `actionitems.Repository.SearchCompleted(ctx context.Context, guildID, query string, limit int) ([]ActionItem, error)`
- Produces: `(*actionitems.Service).SearchCompleted(ctx context.Context, guildID, query string) ([]ActionItem, error)`
- Produces: `actionitems.ErrEmptyQuery`, `actionitems.searchLimit`
- Produces: `postgres.escapeLikePattern(s string) string` (package-private pure helper)
- Consumes: existing `actionitems.ActionItem`, `actionitems.StatusDone`, `postgres.selectColumns`, `postgres.scanItem`

These pieces are tightly coupled — adding the interface method breaks compilation of both implementations until all exist — so they land in one task. The tree is green at the end of this task.

- [ ] **Step 1: Write the failing service tests**

Append to `internal/actionitems/service_test.go`:

```go
func TestSearchCompleted_MatchesSubstringCaseInsensitively(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "Buy Oat Milk", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "exact case", query: "Oat Milk", want: 1},
		{name: "lowercase query", query: "oat milk", want: 1},
		{name: "uppercase query", query: "OAT MILK", want: 1},
		{name: "interior substring", query: "at mi", want: 1},
		{name: "no match", query: "coffee", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.SearchCompleted(ctx, "guild1", tt.query)
			if err != nil {
				t.Fatalf("SearchCompleted() error = %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("len(got) = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestSearchCompleted_OnlyReturnsDoneItems(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	newItem, _ := s.CreateItem(ctx, "guild1", "task alpha", "user1", now)

	inProgress, _ := s.CreateItem(ctx, "guild1", "task beta", "user1", now)
	_ = s.MarkInProgress(ctx, inProgress.ID)

	done, _ := s.CreateItem(ctx, "guild1", "task gamma", "user1", now)
	_ = s.CompleteItem(ctx, done.ID, "approver1", now)

	got, err := s.SearchCompleted(ctx, "guild1", "task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != done.ID {
		t.Errorf("got[0].ID = %q, want %q (the done item)", got[0].ID, done.ID)
	}
	_ = newItem
}

func TestSearchCompleted_ScopedToGuild(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	mine, _ := s.CreateItem(ctx, "guild1", "shared description", "user1", now)
	_ = s.CompleteItem(ctx, mine.ID, "approver1", now)

	theirs, _ := s.CreateItem(ctx, "guild2", "shared description", "user1", now)
	_ = s.CompleteItem(ctx, theirs.ID, "approver1", now)

	got, err := s.SearchCompleted(ctx, "guild1", "shared")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != mine.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, mine.ID)
	}
}

func TestSearchCompleted_NoTimeWindow(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	ancient, _ := s.CreateItem(ctx, "guild1", "very old task", "user1", now)
	_ = s.CompleteItem(ctx, ancient.ID, "approver1", now.Add(-365*24*time.Hour))

	got, err := s.SearchCompleted(ctx, "guild1", "old task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (search has no time cutoff)", len(got))
	}
}

func TestSearchCompleted_CapsAtTenMostRecentFirst(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	for n := 0; n < 12; n++ {
		item, _ := s.CreateItem(ctx, "guild1", "task", "user1", now)
		// n == 11 is the most recently completed.
		_ = s.CompleteItem(ctx, item.ID, "approver1", now.Add(time.Duration(n)*time.Minute))
	}

	got, err := s.SearchCompleted(ctx, "guild1", "task")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want 10", len(got))
	}
	for idx := 1; idx < len(got); idx++ {
		if got[idx].CompletedAt.After(*got[idx-1].CompletedAt) {
			t.Fatalf("results not ordered most-recently-completed first at index %d", idx)
		}
	}
	if !got[0].CompletedAt.Equal(now.Add(11 * time.Minute)) {
		t.Errorf("got[0].CompletedAt = %v, want the newest completion %v", got[0].CompletedAt, now.Add(11*time.Minute))
	}
}

func TestSearchCompleted_RejectsBlankQuery(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	for _, query := range []string{"", "   ", "\t\n"} {
		_, err := s.SearchCompleted(ctx, "guild1", query)
		if !errors.Is(err, ErrEmptyQuery) {
			t.Errorf("SearchCompleted(%q) err = %v, want ErrEmptyQuery", query, err)
		}
	}
}

func TestSearchCompleted_TrimsSurroundingWhitespace(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	now := time.Now()

	item, _ := s.CreateItem(ctx, "guild1", "buy milk", "user1", now)
	_ = s.CompleteItem(ctx, item.ID, "approver1", now)

	got, err := s.SearchCompleted(ctx, "guild1", "  milk  ")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}

func TestSearchCompleted_NoMatchesReturnsEmptyNotError(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	got, err := s.SearchCompleted(ctx, "guild1", "nothing here")
	if err != nil {
		t.Fatalf("SearchCompleted() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
```

No new imports are needed — `context`, `errors`, `testing`, and `time` are already imported by this file.

- [ ] **Step 2: Run the tests and confirm they fail to compile**

Run: `go test ./internal/actionitems/...`

Expected: FAIL — compile errors, roughly:
```
internal/actionitems/service_test.go:NN:NN: s.SearchCompleted undefined (type *Service has no field or method SearchCompleted)
internal/actionitems/service_test.go:NN:NN: undefined: ErrEmptyQuery
```

- [ ] **Step 3: Add `SearchCompleted` to the Repository interface**

In `internal/actionitems/repository.go`, insert the new method immediately after `ListCompletedSince` so the two read together:

```go
	ListCompletedSince(ctx context.Context, guildID string, since time.Time, limit int) ([]ActionItem, error)
	SearchCompleted(ctx context.Context, guildID, query string, limit int) ([]ActionItem, error)
	Reopen(ctx context.Context, id, newMessageID string, restoreStatus Status) error
```

Also extend the interface's doc comment so the matching contract is stated once, at the interface, rather than duplicated in two implementations. Replace the existing comment block above `type Repository interface` with:

```go
// Repository persists ActionItems and per-guild configuration.
// Implementations must return ErrNotFound when an ActionItem lookup finds no
// matching row. GetGuildConfig must never return ErrNotFound — an
// unconfigured guild gets a zero-value config with default emotes filled in.
// SearchCompleted matches query as a case-insensitive substring of the
// description across all done items in the guild, newest completion first,
// treating every character in query — including LIKE wildcards — literally.
type Repository interface {
```

- [ ] **Step 4: Add the fake implementation**

In `internal/actionitems/fake_repository_test.go`, add `"strings"` to the import block:

```go
import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)
```

and add the method directly after `ListCompletedSince`:

```go
func (f *fakeRepository) SearchCompleted(_ context.Context, guildID, query string, limit int) ([]ActionItem, error) {
	needle := strings.ToLower(query)
	var result []ActionItem
	for _, item := range f.items {
		if item.GuildID != guildID || item.Status != StatusDone || item.CompletedAt == nil {
			continue
		}
		if !strings.Contains(strings.ToLower(item.Description), needle) {
			continue
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CompletedAt.After(*result[j].CompletedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
```

(`strings.Contains` on lowercased text is inherently literal, which is exactly the semantics the escaped `ILIKE` gives in Postgres — the fake and the real store agree.)

- [ ] **Step 5: Add the service method, limit constant, and sentinel error**

In `internal/actionitems/service.go`, add `"strings"` to the imports:

```go
import (
	"context"
	"errors"
	"strings"
	"time"
)
```

Extend the const block:

```go
const (
	undoWindow  = 24 * time.Hour
	undoLimit   = 5
	searchLimit = 10
)
```

Extend the error block:

```go
var (
	ErrNotUndoable       = errors.New("action item is not eligible for undo")
	ErrInvalidTransition = errors.New("action item is not in a state that allows this transition")
	ErrInvalidEmote      = errors.New("emote must not be empty")
	ErrEmptyQuery        = errors.New("search query must not be empty")
)
```

And add the method immediately after `ListUndoable`, keeping the completed-items methods together:

```go
// SearchCompleted returns up to searchLimit done action items in the guild
// whose description contains query (case-insensitive), most recently
// completed first. Unlike ListUndoable it searches the full history with no
// time cutoff.
func (s *Service) SearchCompleted(ctx context.Context, guildID, query string) ([]ActionItem, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, ErrEmptyQuery
	}
	return s.repo.SearchCompleted(ctx, guildID, trimmed, searchLimit)
}
```

- [ ] **Step 6: Run the service tests and confirm they pass**

Run: `go test ./internal/actionitems/... -run TestSearchCompleted -v`

Expected: PASS — `TestSearchCompleted_MatchesSubstringCaseInsensitively` (5 subtests), `_OnlyReturnsDoneItems`, `_ScopedToGuild`, `_NoTimeWindow`, `_CapsAtTenMostRecentFirst`, `_RejectsBlankQuery`, `_TrimsSurroundingWhitespace`, `_NoMatchesReturnsEmptyNotError`.

Run: `go build ./...`

Expected: FAIL, and this is the one intentional red moment inside this task:
```
internal/store/postgres/repository.go:22:6: cannot use (*Repository)(nil) (value of type *Repository) as actionitems.Repository value in variable declaration: *Repository does not implement actionitems.Repository (missing method SearchCompleted)
```
Steps 7-9 close it.

- [ ] **Step 7: Write the failing wildcard-escaping unit test**

Create `internal/store/postgres/repository_test.go` — a plain (non-`integration`-tagged) test file, because `escapeLikePattern` is a pure function that needs no database:

```go
package postgres

import "testing"

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text is unchanged", input: "buy milk", want: "buy milk"},
		{name: "empty string", input: "", want: ""},
		{name: "percent is escaped", input: "50% off", want: `50\% off`},
		{name: "underscore is escaped", input: "snake_case", want: `snake\_case`},
		{name: "backslash is escaped", input: `back\slash`, want: `back\\slash`},
		{name: "all metacharacters together", input: `_100%\`, want: `\_100\%\\`},
		{name: "sql-ish punctuation is left alone", input: `'; DROP TABLE action_items; --`, want: `'; DROP TABLE action_items; --`},
		{name: "multibyte text is preserved", input: "café ☕", want: "café ☕"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLikePattern(tt.input); got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

Run: `go test ./internal/store/postgres/...`

Expected: FAIL — `undefined: escapeLikePattern` (plus the pre-existing interface-satisfaction error from Step 6).

- [ ] **Step 8: Implement `escapeLikePattern` and `SearchCompleted` in the postgres repository**

In `internal/store/postgres/repository.go`, add `"strings"` to the import block:

```go
import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)
```

Add the helper and its constant directly above `SearchCompleted`, and insert `SearchCompleted` immediately after `ListCompletedSince` (mirroring the interface ordering):

```go
// likeEscapeChar is the escape character used with the ESCAPE clause in
// SearchCompleted. It must stay in sync with that query's ESCAPE '\'.
const likeEscapeChar = '\\'

// escapeLikePattern escapes the ILIKE metacharacters '%' and '_', plus the
// escape character itself, so user-supplied search text matches literally.
// This is a correctness measure only — injection is prevented by passing the
// pattern as a bind parameter, never by string concatenation.
func escapeLikePattern(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == likeEscapeChar {
			b.WriteRune(likeEscapeChar)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (r *Repository) SearchCompleted(ctx context.Context, guildID, query string, limit int) ([]actionitems.ActionItem, error) {
	pattern := "%" + escapeLikePattern(query) + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT `+selectColumns+` FROM action_items
		WHERE guild_id = $1 AND status = $2 AND description ILIKE $3 ESCAPE '\'
		ORDER BY completed_at DESC
		LIMIT $4`,
		guildID, string(actionitems.StatusDone), pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("searching completed action items: %w", err)
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
```

Two details that matter and are easy to get wrong:
- The SQL is a Go **raw string literal** (backticks), so `ESCAPE '\'` is a single literal backslash reaching Postgres. Do not double it.
- Postgres runs with `standard_conforming_strings = on` by default, so `'\'` is a one-character string, not an escape sequence. No `E''` prefix.

- [ ] **Step 9: Run the unit tests and the build**

Run: `go test ./internal/store/postgres/... -v`

Expected: PASS — `TestEscapeLikePattern` with all 8 subtests. (Only the untagged file runs here; integration tests still require `-tags=integration`.)

Run: `go build ./... && go vet ./...`

Expected: clean, no output. The interface-satisfaction error from Step 6 is gone.

Run: `go test ./...`

Expected: PASS across `internal/actionitems`, `internal/config`, `internal/discord`, `internal/health`, `internal/store/postgres`.

- [ ] **Step 10: Write the Postgres integration tests, including the injection test**

Append to `internal/store/postgres/repository_integration_test.go`:

```go
func TestRepository_SearchCompleted_MatchesSubstringCaseInsensitivelyDoneOnly(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	done, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "Buy Oat Milk", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	if err := repo.Complete(ctx, done.ID, "approver1", now, actionitems.StatusNew); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Same words, but never completed — must not appear.
	if _, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "buy oat milk again", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.SearchCompleted(ctx, "guild1", "OAT MILK", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != done.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, done.ID)
	}

	none, err := repo.SearchCompleted(ctx, "guild1", "coffee", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("len(none) = %d, want 0", len(none))
	}
}

func TestRepository_SearchCompleted_ScopedToGuild(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	mine, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "shared description", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, mine.ID, "approver1", now, actionitems.StatusNew)

	theirs, _ := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild2", Description: "shared description", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	_ = repo.Complete(ctx, theirs.ID, "approver1", now, actionitems.StatusNew)

	got, err := repo.SearchCompleted(ctx, "guild1", "shared", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != mine.ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, mine.ID)
	}
}

func TestRepository_SearchCompleted_NewestFirstAndRespectsLimit(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	var newestID string
	for n := 0; n < 12; n++ {
		item, err := repo.Create(ctx, actionitems.ActionItem{
			GuildID: "guild1", Description: "task", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Complete(ctx, item.ID, "approver1", now.Add(time.Duration(n)*time.Minute), actionitems.StatusNew); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		newestID = item.ID
	}

	got, err := repo.SearchCompleted(ctx, "guild1", "task", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want 10", len(got))
	}
	if got[0].ID != newestID {
		t.Errorf("got[0].ID = %q, want the most recently completed item %q", got[0].ID, newestID)
	}
	for idx := 1; idx < len(got); idx++ {
		if got[idx].CompletedAt.After(*got[idx-1].CompletedAt) {
			t.Fatalf("results not ordered newest-first at index %d", idx)
		}
	}
}

func TestRepository_SearchCompleted_TreatsLikeWildcardsAsLiterals(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	create := func(description string) string {
		t.Helper()
		item, err := repo.Create(ctx, actionitems.ActionItem{
			GuildID: "guild1", Description: description, CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
		})
		if err != nil {
			t.Fatalf("Create(%q): %v", description, err)
		}
		if err := repo.Complete(ctx, item.ID, "approver1", now, actionitems.StatusNew); err != nil {
			t.Fatalf("Complete(%q): %v", description, err)
		}
		return item.ID
	}

	percentID := create("50% off coupon")
	create("50 off coupon")
	underscoreID := create("snake_case name")
	create("snakeXcase name")

	// "%" must not act as a wildcard: "50%" matches only the literal "50% ...".
	got, err := repo.SearchCompleted(ctx, "guild1", "50%", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 1 || got[0].ID != percentID {
		t.Fatalf("searching \"50%%\" returned %d results, want only the literal-percent item", len(got))
	}

	// "_" must not act as a single-character wildcard.
	got, err = repo.SearchCompleted(ctx, "guild1", "snake_case", 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 1 || got[0].ID != underscoreID {
		t.Fatalf("searching \"snake_case\" returned %d results, want only the literal-underscore item", len(got))
	}

	// A lone backslash must not produce a malformed pattern error.
	if _, err := repo.SearchCompleted(ctx, "guild1", `\`, 10); err != nil {
		t.Fatalf("SearchCompleted with a lone backslash: %v", err)
	}
}

func TestRepository_SearchCompleted_TreatsSQLInjectionAttemptAsInertText(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()

	const injection = `'; DROP TABLE action_items; --`

	existing, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: "buy milk", CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Complete(ctx, existing.ID, "approver1", now, actionitems.StatusNew); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := repo.SearchCompleted(ctx, "guild1", injection, 10)
	if err != nil {
		t.Fatalf("SearchCompleted with injection text: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0 — injection text should match no descriptions", len(got))
	}

	// The table must still exist, still be queryable, and still hold its row.
	if _, err := repo.Get(ctx, existing.ID); err != nil {
		t.Fatalf("Get after injection attempt: %v (action_items may have been dropped)", err)
	}
	var count int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM action_items`).Scan(&count); err != nil {
		t.Fatalf("counting action_items after injection attempt: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// And the same text, stored as a description, is findable — proving the
	// query text is treated as ordinary search text end to end.
	stored, err := repo.Create(ctx, actionitems.ActionItem{
		GuildID: "guild1", Description: injection, CreatedByUserID: "user1", CreatedAt: now, Status: actionitems.StatusNew,
	})
	if err != nil {
		t.Fatalf("Create with injection text as description: %v", err)
	}
	if err := repo.Complete(ctx, stored.ID, "approver1", now.Add(time.Minute), actionitems.StatusNew); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err = repo.SearchCompleted(ctx, "guild1", injection, 10)
	if err != nil {
		t.Fatalf("SearchCompleted: %v", err)
	}
	if len(got) != 1 || got[0].ID != stored.ID {
		t.Fatalf("len(got) = %d, want exactly the item whose description is the literal injection string", len(got))
	}
	if _, err := repo.Get(ctx, existing.ID); err != nil {
		t.Fatalf("Get after second injection attempt: %v", err)
	}
}
```

No new imports are required — `context`, `os`, `testing`, `time`, and `actionitems` are already imported.

- [ ] **Step 11: Run the integration tests against real Postgres**

Run:
```bash
docker compose up -d postgres
go test -tags=integration ./internal/store/postgres/... -v
```

Expected: PASS for all pre-existing tests plus `TestRepository_SearchCompleted_MatchesSubstringCaseInsensitivelyDoneOnly`, `_ScopedToGuild`, `_NewestFirstAndRespectsLimit`, `_TreatsLikeWildcardsAsLiterals`, and `_TreatsSQLInjectionAttemptAsInertText`.

If `_TreatsSQLInjectionAttemptAsInertText` fails at the `Get after injection attempt` assertion, stop — that would mean the query was not parameterized. Re-read `SearchCompleted` and confirm the pattern is passed as `$3` and nothing was concatenated.

- [ ] **Step 12: Commit**

```bash
git add internal/actionitems/repository.go internal/actionitems/fake_repository_test.go \
        internal/actionitems/service.go internal/actionitems/service_test.go \
        internal/store/postgres/repository.go internal/store/postgres/repository_test.go \
        internal/store/postgres/repository_integration_test.go
git commit -m "feat: add guild-scoped completed action item search to repository and service"
```

---

### Task 2: `/search` slash command

**Files:**
- Modify: `internal/discord/helpers.go`
- Modify: `internal/discord/helpers_test.go`
- Modify: `internal/discord/bot.go`
- Modify: `internal/discord/commands.go`
- Modify: `internal/discord/help_message.go`
- Modify: `internal/discord/help_message_test.go`

**Interfaces:**
- Produces: `searchQueryText(options []*discordgo.ApplicationCommandInteractionDataOption) string`
- Produces: `truncateDescription(s string, max int) string`
- Produces: `searchResultsText(items []actionitems.ActionItem) string`
- Produces: `(*Bot).handleSearchCommand(s *discordgo.Session, i *discordgo.InteractionCreate)` (untested glue)
- Consumes: `(*actionitems.Service).SearchCompleted`, `actionitems.ErrEmptyQuery`, `respondEphemeral`

- [ ] **Step 1: Write the failing helper tests**

Append to `internal/discord/helpers_test.go`:

```go
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
	if strings.Contains(got, "completed ") {
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
```

`strings`, `testing`, `time`, `discordgo`, and `actionitems` are already imported by this file.

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/discord/...`

Expected: FAIL — compile errors:
```
internal/discord/helpers_test.go:NN:NN: undefined: searchQueryText
internal/discord/helpers_test.go:NN:NN: undefined: truncateDescription
internal/discord/helpers_test.go:NN:NN: undefined: searchResultsText
```

- [ ] **Step 3: Implement the pure helpers**

In `internal/discord/helpers.go`, add a constant and the three helpers, and refactor `undoSelectOptions` onto the shared truncation helper so the 100-character rule exists once.

Add near the top of the file, after the imports:

```go
// maxDisplayedDescription bounds how much of a description is echoed back in
// select-menu labels and search results, keeping responses well under
// Discord's 2000-character message limit.
const maxDisplayedDescription = 100
```

Replace `undoSelectOptions` with:

```go
func undoSelectOptions(items []actionitems.ActionItem) []discordgo.SelectMenuOption {
	options := make([]discordgo.SelectMenuOption, 0, len(items))
	for _, item := range items {
		options = append(options, discordgo.SelectMenuOption{
			Label:       truncateDescription(item.Description, maxDisplayedDescription),
			Value:       item.ID,
			Description: fmt.Sprintf("completed %s", item.CompletedAt.Format(time.RFC822)),
		})
	}
	return options
}
```

Add `searchQueryText` directly after `actionItemText` (deliberately mirroring it rather than refactoring `actionItemText` into a generic option reader — this keeps existing, already-tested code untouched):

```go
func searchQueryText(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, opt := range options {
		if opt.Name == "query" {
			return opt.StringValue()
		}
	}
	return ""
}
```

Add the truncation helper and the result formatter after `approverListText`:

```go
func truncateDescription(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func searchResultsText(items []actionitems.ActionItem) string {
	if len(items) == 0 {
		return "No completed action items matched that search."
	}

	lines := make([]string, 0, len(items)+1)
	if len(items) == 1 {
		lines = append(lines, "Found 1 completed action item:")
	} else {
		lines = append(lines, fmt.Sprintf("Found %d completed action items:", len(items)))
	}
	for _, item := range items {
		description := truncateDescription(item.Description, maxDisplayedDescription)
		if item.CompletedAt == nil {
			lines = append(lines, fmt.Sprintf("- %s", description))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (completed %s)", description, item.CompletedAt.Format(time.RFC822)))
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run the discord tests and confirm they pass**

Run: `go test ./internal/discord/... -v`

Expected: PASS — the new `TestSearchQueryText_*`, `TestTruncateDescription`, and `TestSearchResultsText_*` tests, plus every pre-existing test including `TestUndoSelectOptions_TruncatesLongDescriptions` (unchanged behavior after the refactor).

- [ ] **Step 5: Register the `/search` command**

In `internal/discord/bot.go`, insert this literal into the `commands` slice immediately after the `undo` entry:

```go
		{
			Name:        "search",
			Description: "Search completed action items",
			Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Text to look for in completed action items",
					Required:    true,
				},
			},
		},
```

Deliberately **no** `DefaultMemberPermissions` field: leaving it unset means the command defaults to allowed-for-everyone and each guild tunes access in Server Settings → Integrations, which is the agreed permission model.

- [ ] **Step 6: Add the dispatch entry and the handler**

In `internal/discord/commands.go`, add `"errors"` to the import block:

```go
import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
)
```

Add the dispatch case in `handleInteraction`, after the `undo` case:

```go
		case "undo":
			b.handleUndoCommand(s, i)
		case "search":
			b.handleSearchCommand(s, i)
		case "approver":
			b.handleApproverCommand(s, i)
```

Add the handler after `handleUndoSelect` (before `handleApproverCommand`):

```go
// handleSearchCommand answers with the guild's matching completed action
// items. Unlike /undo, /approver and /config it performs no approver check —
// access is governed by each guild's Discord Integrations settings, the same
// as /action-item.
func (b *Bot) handleSearchCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	query := searchQueryText(i.ApplicationCommandData().Options)

	items, err := b.service.SearchCompleted(ctx, i.GuildID, query)
	if errors.Is(err, actionitems.ErrEmptyQuery) {
		_ = respondEphemeral(s, i, "Please provide something to search for.")
		return
	}
	if err != nil {
		log.Printf("searching completed action items: %v", err)
		_ = respondEphemeral(s, i, "Failed to search completed action items.")
		return
	}

	_ = respondEphemeral(s, i, searchResultsText(items))
}
```

This handler is glue by the project's convention (it calls `*discordgo.Session` through `respondEphemeral`), so it gets no unit test — all of its decision-making lives in `searchQueryText`, `Service.SearchCompleted`, and `searchResultsText`, which are tested.

- [ ] **Step 7: Mention `/search` in the pinned help message**

The pinned per-guild help message is the bot's discoverability surface for its command set, so it should list the new command.

In `internal/discord/help_message.go`, insert one line into `helpMessageBody` between the **Undo** and **Configuration** paragraphs:

```go
	b.WriteString("**Undo**: if an item was completed by mistake, run `/undo` within 24 hours (last 5 completions) to restore it.\n\n")
	b.WriteString("**Searching**: run `/search query:\"...\"` to look through completed items — results are shown only to you.\n\n")
	b.WriteString("**Configuration**: run `/config` to change the channel, emotes, or approver role. Manage individual approvers with `/approver add`, `/approver remove`, and `/approver list`.")
```

In `internal/discord/help_message_test.go`, add `"/search"` to the expected-substrings list in `TestHelpMessageBody_ContainsEmotesAndWhoCanAct`:

```go
	for _, want := range []string{"🔄", "✅", "<@owner1>", "<@&role1>", "<@user1>", "<@user2>", "/action-item", "/undo", "/search", "/config", "/approver"} {
```

- [ ] **Step 8: Build, vet, and run the full unit test suite**

Run:
```bash
go build ./...
go vet ./...
go test ./... -v
```

Expected: build and vet clean with no output; all tests PASS across `internal/actionitems`, `internal/config`, `internal/discord`, `internal/health`, `internal/store/postgres`.

- [ ] **Step 9: Commit**

```bash
git add internal/discord/helpers.go internal/discord/helpers_test.go internal/discord/bot.go \
        internal/discord/commands.go internal/discord/help_message.go internal/discord/help_message_test.go
git commit -m "feat: add /search command for completed action item history"
```

---

### Task 3: Full verification pass

**Files:** none (verification only).

- [ ] **Step 1: Run the full unit test suite from a clean cache**

Run: `go clean -testcache && go test ./... -v`

Expected: PASS everywhere. Confirm you can see, by name, `TestSearchCompleted_RejectsBlankQuery`, `TestSearchCompleted_CapsAtTenMostRecentFirst`, `TestEscapeLikePattern`, and `TestSearchResultsText_NoResults` in the output — if any of these are absent, a test file was never wired in.

- [ ] **Step 2: Run the Postgres integration tests against a clean database**

Run:
```bash
docker compose down -v
docker compose up -d postgres
go test -tags=integration ./internal/store/postgres/... -v
```

Expected: PASS for the whole file, and specifically:
- `TestRepository_SearchCompleted_MatchesSubstringCaseInsensitivelyDoneOnly`
- `TestRepository_SearchCompleted_ScopedToGuild`
- `TestRepository_SearchCompleted_NewestFirstAndRespectsLimit`
- `TestRepository_SearchCompleted_TreatsLikeWildcardsAsLiterals`
- `TestRepository_SearchCompleted_TreatsSQLInjectionAttemptAsInertText`

`TreatsSQLInjectionAttemptAsInertText` is the test that actually demonstrates the security requirement: the `'; DROP TABLE action_items; --` search returns zero rows, `action_items` still exists and still holds its row afterward, and the same string stored as a description is findable as ordinary text.

- [ ] **Step 3: Grep the new SQL for concatenation, as a belt-and-braces check**

Run: `grep -n "Sprintf" internal/store/postgres/repository.go`

Expected: **no matches** inside any query construction — every value in `SearchCompleted` reaches Postgres as `$1`–`$4`. (`fmt.Errorf` calls are fine and expected; `fmt.Sprintf` should not appear at all in this file.)

- [ ] **Step 4: Build and smoke-test the bot container**

Run:
```bash
docker compose build bot
docker compose up bot
```

Expected: fails fast with `loading config: DISCORD_TOKEN is required` (no real `.env` present) — the same expected failure shape as the existing deployment smoke test, confirming nothing about startup or dependency ordering changed.

Run: `docker compose rm -f bot`

- [ ] **Step 5: Record the manual, credential-requiring checks**

These need a real Discord bot token and a test guild, so they cannot be automated here — hand them to the user:

- Confirm `/search` appears in the guild's command list. Note that global command registration can take up to an hour to propagate the first time a new command is added; a missing command right after deploy is expected, not a bug.
- Complete a few items (via the done emote), then run `/search query:"<a word from one of them>"` — confirm the response is ephemeral (only you see it), lists matching items newest-first, and shows completion timestamps.
- Run `/search query:"zzzzz"` — confirm the plain "No completed action items matched that search." ephemeral reply.
- Confirm items that are still `new`/`in_progress` never appear in results, and that an item completed months ago **does** appear (no 24-hour window, unlike `/undo`).
- Run `/search` as a **non-approver** member — confirm it works, proving the command is not gated by `isOwnerOrApprover`. Then in Server Settings → Integrations, restrict `/search` to a role and confirm Discord itself enforces that.
- Run `/search query:"50%"` and `/search query:"a_b"` on items containing those literal characters — confirm literal matching, not wildcard behavior.
- Confirm the pinned help message now lists `/search` and updated in place (rather than being reposted) after a config change triggered `syncHelpMessage`.
- Confirm a second guild's completed items never appear in the first guild's search results.

- [ ] **Step 6: Report status to the user**

Summarize: unit tests, Postgres integration tests (including the injection test), `go build`, and `go vet` all clean; list the Step 5 manual checks as the remaining verification that requires real Discord credentials.

---

## Self-Review Notes

- **Approved-design coverage**: one required `query` string option (Task 2 Step 5); no permission check (Task 2 Step 6, plus a Global Constraint and a code comment so a future reader doesn't "fix" it); `done`-only + guild-scoped + no time window (Task 1 Steps 5/8, tested at both service and Postgres layers); case-insensitive substring (`ILIKE` / `strings.Contains` on lowercase); 10-result cap newest-first (`searchLimit`, `ORDER BY completed_at DESC LIMIT $4`); always ephemeral with a plain no-results message (`respondEphemeral` + `searchResultsText`); bound parameters plus escaped wildcards with an explicit `ESCAPE` clause (Task 1 Step 8, proved by Task 1 Steps 10-11).
- **Signature consistency verified across all three sites**: `SearchCompleted(ctx context.Context, guildID, query string, limit int) ([]ActionItem, error)` is identical in the interface (Task 1 Step 3), the fake (Step 4), and the postgres implementation (Step 8); the service wrapper deliberately drops `limit` and supplies `searchLimit`, exactly as `ListUndoable` supplies `undoWindow`/`undoLimit` to `ListCompletedSince`.
- **Build-green ordering**: Task 1 ends green and Task 2 ends green. The only red interval is *inside* Task 1 (Steps 1-8), which is unavoidable — adding a method to `actionitems.Repository` breaks `var _ actionitems.Repository = (*Repository)(nil)` in the postgres package until the implementation lands. Step 6 calls this out explicitly with the exact expected compiler error so the executor doesn't mistake it for a mistake.
- **Escape-character coupling flagged**: `likeEscapeChar` in Go and `ESCAPE '\'` in the SQL must agree, and the SQL is a backtick raw string (so the backslash is literal) executed against Postgres with `standard_conforming_strings = on` (so `'\'` is a one-character string). Both facts are stated at the point of implementation because getting either wrong produces a subtle, test-visible-only-at-integration-time bug.
- **Deliberate non-refactor**: `searchQueryText` duplicates the six-line shape of `actionItemText` rather than both delegating to a generic `stringOptionValue(options, name)`. This was a judgment call — the duplication is trivial and the plan explicitly follows the existing pattern, avoiding churn in already-tested code. `truncateDescription`, by contrast, *is* factored out and `undoSelectOptions` refactored onto it, because that logic (the 100-char + ellipsis rule) is a real behavioral rule that should have one definition; the existing `TestUndoSelectOptions_TruncatesLongDescriptions` test guards the refactor.
- **Known soft spots, stated rather than hidden**: (1) `truncateDescription` slices by bytes, so a 100-byte boundary can split a multibyte rune — this exactly preserves today's `undoSelectOptions` behavior, and fixing it is a separate concern affecting both call sites. (2) `ORDER BY completed_at DESC` has no tiebreaker, matching `ListCompletedSince`; results with identical microsecond completion timestamps have unspecified relative order. Integration tests use distinct timestamps so this is not flaky. (3) `ILIKE '%…%'` cannot use an index; at current per-guild volumes this is a bounded sequential scan behind `idx_action_items_guild_id`, and the `pg_trgm` upgrade path is noted in Global Constraints as explicitly out of scope so nobody invents a migration mid-implementation.
- **No placeholders**: every step contains real, complete code, real commands, and concrete expected output — including the exact compiler errors expected at each red step.
