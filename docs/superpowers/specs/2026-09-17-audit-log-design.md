# Discord Action Items Bot — Permission Audit Log

## Context

`BOT_ADMIN_IDS` (see `internal/config`) lets a configured list of Discord user IDs act as the owner-equivalent in *every* guild the bot serves, regardless of who actually owns that guild. This is a deliberate, low-friction override for the bot's operator, but it means privileged actions can now happen without any trace in Discord's own audit log (which only ever attributes actions to the bot's own user, not the human behind a slash command or reaction). This design adds a database-backed audit trail so it's possible to answer "who did this privileged action, and were they the guild owner, a configured approver, or the bot operator overriding both?" after the fact.

This is additive to the existing architecture (discord → actionitems service → postgres repository) — see `docs/superpowers/specs/2026-08-24-multi-tenant-state-machine-design.md` for that baseline, which carries forward unchanged.

## Decisions

- **Scope: allowed actions only, not denials.** `isOwnerOrApprover` is checked on every relevant status reaction, not just explicit commands, so logging every denial would record routine unauthorized-reaction noise (e.g. a non-approver reacting with the done emote) rather than meaningful admin activity. Only successful permission grants are recorded.
- **Reason is recorded per entry**: `bot_admin`, `guild_owner`, or `approver` — one of the three paths `isOwnerOrApprover` already distinguishes internally. This is what makes bot-admin usage specifically traceable, separate from a guild's own owner or approvers acting normally.
- **Single central logging point**: rather than duplicating audit-write calls at each of the 10 existing call sites that gate on `isOwnerOrApprover`/`requireOwnerOrApprover` (5 in `config_panel.go`, 3 in `commands.go`, 2 in `reactions.go`), the function itself records the entry on each allow path. Call sites only gain a new `action string` argument identifying what was being attempted (e.g. `"config.set_channel"`, `"approver.add"`, `"reaction.mark_done"`) — no new logic duplicated per site.
- **Storage only for now, no viewer command.** Entries are queryable directly via Postgres (`psql`/DB tooling). A `/audit-log` command or similar can be added later if needed — YAGNI for this change.
- **New `internal/audit` package**, separate from `internal/actionitems`. An audit entry isn't action-item domain data — it's a record of a Discord-layer permission decision — so it gets its own small `Entry` type and `Repository` interface rather than growing the `actionitems.Repository` interface (interface segregation; the actionitems layer has no reason to know about audit entries).
- **Same Postgres connection pool, no new repository struct.** The existing `postgres.Repository` (already wrapping the pool, already implementing `actionitems.Repository`) gains a `Record` method implementing `audit.Repository`, in a new file `internal/store/postgres/audit.go`. `main.go` passes the same `repo` value it already builds to both `actionitems.NewService` and the new `discord.New` parameter — no second pool, no second exported type.

## Data model changes

New migration `0003_audit_log` (up/down):

**`audit_log`** (append-only):
| column | type | notes |
|---|---|---|
| id | bigserial (pk) | |
| guild_id | text | |
| user_id | text | the Discord user who took the action |
| action | text | short dotted identifier, e.g. `approver.add` |
| reason | text | one of `bot_admin`, `guild_owner`, `approver` |
| created_at | timestamptz | |

Indexes on `guild_id` and `created_at` (typical query shapes: "recent activity in this guild", "recent activity overall").

## Package: `internal/audit`

```go
package audit

type Reason string

const (
    ReasonBotAdmin   Reason = "bot_admin"
    ReasonGuildOwner Reason = "guild_owner"
    ReasonApprover   Reason = "approver"
)

type Entry struct {
    GuildID   string
    UserID    string
    Action    string
    Reason    Reason
    CreatedAt time.Time
}

type Repository interface {
    Record(ctx context.Context, entry Entry) error
}
```

No `Service` layer — there's no business logic beyond persisting the entry, so `Bot` holds an `audit.Repository` directly.

## Wiring

- `Bot` struct gains `auditLog audit.Repository`.
- `discord.New(token string, service *actionitems.Service, botAdminIDs []string, auditLog audit.Repository) (*Bot, error)` — new parameter appended.
- `cmd/bot/main.go` passes the existing `repo` (a `*postgres.Repository`, which now also satisfies `audit.Repository`).

## Permission check changes

`isOwnerOrApprover(ctx, guildID, action string, member)` and `requireOwnerOrApprover(ctx, s, i, action, denyMsg)` both gain the `action` parameter, threaded through from each of the 10 call sites using short constants defined in `internal/discord/permissions.go`:

- `config.open`, `config.set_channel`, `config.set_role`, `config.edit_emotes_button`, `config.save_emotes`
- `approver.add`, `approver.remove`, `approver.list`
- `undo.list`, `undo.select`
- `reaction.mark_in_progress`, `reaction.mark_done`, `reaction.mark_new`

On each of the three allow paths (bot admin short-circuit, guild-owner match, approver match), `isOwnerOrApprover` calls a small `recordAudit` helper with the corresponding `Reason` before returning `true`. A failure to write the audit entry is logged (via the existing `log.Printf` pattern) but does not block the action — audit logging is best-effort observability, not a gate. If `auditLog` is nil (e.g. in unit tests that construct a `Bot` directly), `recordAudit` is a no-op.

## Testing

- `internal/discord/permissions_test.go`: extend the existing bot-admin test to assert a `bot_admin`-reason entry is recorded via a fake `audit.Repository`; add a guild-owner-path test seeding `discordgo.State.GuildAdd` (no network needed) and asserting a `guild_owner`-reason entry.
- `internal/store/postgres`: an integration test (behind the existing `integration` build tag) verifying `Record` persists a row with the expected columns, following the pattern in `repository_integration_test.go`.
- `cmd/bot/main.go` wiring has no test today (none of its other wiring does either) — unchanged in that respect.
