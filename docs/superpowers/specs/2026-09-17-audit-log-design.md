# Discord Action Items Bot — Permission Audit Log

## Context

`BOT_ADMIN_IDS` (see `internal/config`) lets a configured list of Discord user IDs act as the owner-equivalent in *every* guild the bot serves, regardless of who actually owns that guild. This is a deliberate, low-friction override for the bot's operator, but it means privileged actions can now happen without any trace in Discord's own audit log (which only ever attributes actions to the bot's own user, not the human behind a slash command or reaction). This design adds a database-backed audit trail so it's possible to answer "who did this privileged action, and were they the guild owner, a configured approver, or the bot operator overriding both?" after the fact.

This is additive to the existing architecture (discord → actionitems service → postgres repository) — see `docs/superpowers/specs/2026-08-24-multi-tenant-state-machine-design.md` for that baseline, which carries forward unchanged.

## Decisions

- **Scope: allowed actions only, not denials.** `isOwnerOrApprover` is checked on every relevant status reaction, not just explicit commands, so logging every denial would record routine unauthorized-reaction noise (e.g. a non-approver reacting with the done emote) rather than meaningful admin activity. Only successful permission grants are recorded.
- **Reason is recorded per entry**: `bot_admin`, `guild_owner`, or `approver` — one of the three paths `isOwnerOrApprover` already distinguishes internally. This is what makes bot-admin usage specifically traceable, separate from a guild's own owner or approvers acting normally.
- **Username is snapshotted at write time.** IDs are the durable key, but a display name at time-of-action makes entries scannable without cross-referencing Discord. Since usernames change, this is stored as-was, not live-joined later.
- **No IP address or client metadata.** Discord's Bot API doesn't expose the end user's IP or device/client info to bots for either interactions or reactions — that data only exists on Discord's own infrastructure. The nearest available thing, `Interaction.Locale`, only exists on slash-command/component/modal interactions (not reactions) and isn't worth the inconsistency for this change.
- **No separate Discord-side event timestamp.** A Discord snowflake ID (e.g. `Interaction.ID`) encodes its own creation time, but `MessageReactionAdd`/`Remove` gateway events carry no timestamp at all, so this would only ever be populated for 8 of the 13 tracked actions. `created_at` (when the bot processed the action) is used uniformly instead; gateway/processing lag is normally sub-second.
- **Before/after state, for mutating actions only.** Actions that change something (channel, role, emotes, approver membership, item status) record what changed; actions that only grant a look (opening the config panel, opening the emotes modal, `/approver list`, `/undo` listing) leave both null. Represented as nullable `jsonb` columns holding a flat `map[string]string`, since the shape differs by action (a single ID, a pair of emotes, a status value) but all reduce to string key/value pairs — no per-action-type schema needed, and it stays queryable with Postgres's jsonb operators.
- **Permission check no longer writes the audit entry itself.** Capturing "after" state requires writing *after* the mutation succeeds, which happens later than the permission check. So `isOwnerOrApprover`/`requireOwnerOrApprover` are responsible only for *deciding* allow/deny and returning which of the three reasons applied; each call site writes its own entry (via a shared `recordAudit` helper that does the actual DB call and JSON marshaling) once it knows the action's before/after state, if any. This is a change from treating the permission check as the single write point, but keeps the one part that's genuinely common — the DB write, marshaling, and nil-safety — in one helper.
- **New `internal/audit` package**, separate from `internal/actionitems`. An audit entry isn't action-item domain data — it's a record of a Discord-layer permission decision — so it gets its own small `Entry` type and `Repository` interface rather than growing the `actionitems.Repository` interface (interface segregation; the actionitems layer has no reason to know about audit entries).
- **Same Postgres connection pool, no new repository struct.** The existing `postgres.Repository` (already wrapping the pool, already implementing `actionitems.Repository`) gains a `Record` method implementing `audit.Repository`, in a new file `internal/store/postgres/audit.go`. `main.go` passes the same `repo` value it already builds to both `actionitems.NewService` and the new `discord.New` parameter — no second pool, no second exported type.
- **Storage only for now, no viewer command.** Entries are queryable directly via Postgres (`psql`/DB tooling). A `/audit-log` command or similar can be added later if needed — YAGNI for this change.

## Data model changes

New migration `0003_audit_log` (up/down):

**`audit_log`** (append-only):
| column | type | notes |
|---|---|---|
| id | bigserial (pk) | |
| guild_id | text | |
| user_id | text | the Discord user who took the action |
| username | text | snapshot of their Discord username at write time |
| action | text | short dotted identifier, e.g. `approver.add` |
| reason | text | one of `bot_admin`, `guild_owner`, `approver` |
| before_state | jsonb, nullable | flat string map; null for view-only actions |
| after_state | jsonb, nullable | flat string map; null for view-only actions |
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
    Username  string
    Action    string
    Reason    Reason
    Before    map[string]string // nil when the action has no meaningful before state
    After     map[string]string // nil when the action has no meaningful after state
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

`isOwnerOrApprover(ctx, guildID, member) (allowed bool, reason audit.Reason, err error)` gains a return value (the matched reason; zero value when denied or on error) but no new parameters — it stays purely about the allow/deny decision. `requireOwnerOrApprover` passes the reason back out the same way, alongside its existing bool.

A shared helper does the actual write:

```go
func (b *Bot) recordAudit(ctx context.Context, guildID string, member *discordgo.Member, action string, reason audit.Reason, before, after map[string]string) {
    if b.auditLog == nil {
        return
    }
    err := b.auditLog.Record(ctx, audit.Entry{
        GuildID:   guildID,
        UserID:    member.User.ID,
        Username:  member.User.Username,
        Action:    action,
        Reason:    reason,
        Before:    before,
        After:     after,
        CreatedAt: time.Now(),
    })
    if err != nil {
        log.Printf("recording audit log entry: %v", err)
    }
}
```

A failure to write is logged but never blocks the action — audit logging is best-effort observability, not a gate. If `auditLog` is nil (e.g. unit tests constructing a `Bot` directly), `recordAudit` is a no-op.

Each of the 10 permission-check call sites calls `recordAudit` once it knows the outcome, using one of 13 distinct action identifiers (a couple of call sites cover more than one action depending on which branch is taken — e.g. `handleApproverCommand` gates once but logs `approver.add`, `approver.remove`, or `approver.list` depending on the subcommand; `handleReactionAdd` gates once but logs `reaction.mark_in_progress` or `reaction.mark_done` depending on the target status):

| action | site | before | after |
|---|---|---|---|
| `config.open` | `handleConfigCommand` | — | — |
| `config.set_channel` | `handleConfigChannelSelect` | `{"channel_id": <old>}` | `{"channel_id": <new>}` |
| `config.set_role` | `handleConfigRoleSelect` | `{"role_id": <old>}` | `{"role_id": <new>}` |
| `config.edit_emotes_button` | `handleConfigEditEmotesButton` | — | — |
| `config.save_emotes` | `handleConfigEmotesModalSubmit` | `{"in_progress_emote": <old>, "done_emote": <old>}` | `{"in_progress_emote": <new>, "done_emote": <new>}` |
| `approver.add` | `handleApproverCommand` (add) | — | `{"user_id": <added>}` |
| `approver.remove` | `handleApproverCommand` (remove) | `{"user_id": <removed>}` | — |
| `approver.list` | `handleApproverCommand` (list) | — | — |
| `undo.list` | `handleUndoCommand` | — | — |
| `undo.select` | `handleUndoSelect` | `{"status": <item.Status>}` | `{"status": <restoreStatus>}` |
| `reaction.mark_in_progress` | `handleReactionAdd` | `{"status": "new"}` | `{"status": "in_progress"}` |
| `reaction.mark_done` | `handleReactionAdd` | `{"status": <item.Status>}` | `{"status": "done"}` |
| `reaction.mark_new` | `handleReactionRemove` | `{"status": "in_progress"}` | `{"status": "new"}` |

`config.set_channel` and `config.set_role` need one extra `GetGuildConfig` read before mutating (to capture the "before" value) where the handler doesn't already have it in hand; `config.save_emotes` already fetches the config after saving to refresh the panel, so only the pre-save fetch is new.

## Testing

- `internal/discord/permissions_test.go`: extend the existing bot-admin test to assert `isOwnerOrApprover` returns `ReasonBotAdmin`; add a guild-owner-path test seeding `discordgo.State.GuildAdd` (no network needed) asserting `ReasonGuildOwner`.
- A `recordAudit` test using a fake `audit.Repository` asserting the entry's fields (including that `Before`/`After` come through nil when not passed, and populated when passed).
- `internal/store/postgres`: an integration test (behind the existing `integration` build tag) verifying `Record` persists a row with the expected columns, including a round-trip of `before_state`/`after_state` JSON, following the pattern in `repository_integration_test.go`.
- `cmd/bot/main.go` wiring has no test today (none of its other wiring does either) — unchanged in that respect.
