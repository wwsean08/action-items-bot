# Discord Action Items Bot — Multi-Tenant + State Machine

## Context

The bot was built and tested as a single-tenant, single-guild deployment configured entirely via environment variables. After trying it out, the user's friends want it in their own servers, and the linear "pending → completed" model isn't expressive enough — items need a visible "in progress" state, not just created/done. This requires:

- Moving guild-specific configuration (channel, approvers, emotes) out of environment variables and into the database, managed by in-Discord commands, since one bot process now serves many guilds.
- A three-state lifecycle (`new` → `in_progress` → `done`) with configurable emotes per transition, defaulting to sensible values but overridable per guild.
- A permission model for who can manage a guild's config, since there's no longer a fixed operator-set approver list — it has to bootstrap from something already true in Discord (the guild owner).

This supersedes the original single-tenant design (`docs/superpowers/specs` predecessor, if present). The existing architecture (discord → actionitems service → postgres repository layering, Repository interface + fake for unit tests, TDD approach, health check) all carries forward unchanged in spirit — this describes what's added/changed on top of it.

## Decisions

- **Command scope**: slash commands register globally (guild ID `""`), not per-guild. Works automatically in any server the bot is invited to. Per-guild access control for *who* can invoke `/action-item` stays exactly as before — enforced by each guild's own Integrations settings, not by the bot.
- **State model**: `new` (initial) → `in_progress` → `done`, linear but skippable — reacting with the `done` emote on a `new` item is allowed and jumps straight to done.
- **In-progress is reversible live**: reacting with the configured in-progress emote moves `new → in_progress` and edits the message prefix. *Removing* that reaction moves `in_progress → new` and edits the prefix back. No `/undo` involved — this is symmetric and instant.
- **Done stays destructive**: reacting with the configured done emote deletes the channel message (same as today), and records `previous_status` (whatever state — `new` or `in_progress` — the item was in right before completion).
- **`/undo` narrows to done only**: since the message is gone once `done`, `/undo` is the only way back. It lists the guild's last 5 items completed within 24h (unchanged window/limit) and restores the selected one to its recorded `previous_status`, reposting the message with the matching prefix.
- **Message prefix**: bracketed text label — `[NEW] `, `[IN PROGRESS] ` — prepended to the description. `done` doesn't need a prefix since the message is deleted; `/undo` reposts with whatever prefix matches the restored state.
- **Approvers stay two-shaped**: a per-guild set of individually-added user IDs, plus an optional single per-guild role ID. Both configurable via commands.
- **Bootstrap authority**: the Discord guild owner (checked live via the Discord API, not stored) can always run `/config` and `/approver` commands, even before any approvers exist. Once approvers exist, they can also run these commands — same permission check as reacting to complete an item.
- **Config is lazily created**: the first `/config` or `/approver` command run in a guild upserts that guild's config row (with default emotes, empty channel) rather than requiring a separate `/setup` step.
- **`/config` is an interactive panel, not flag-based commands**: it opens an ephemeral message with native Discord **Channel Select** and **Role Select** components (auto-save on pick) plus an **"Edit Emotes" button** that opens a **modal** for the two emoji fields — the one place a modal is warranted, since emoji entry is free text. `/approver add/remove/list` stay as plain slash commands with a `user` option, since Discord's native user-option picker already gives a good inline picker and these are explicit, auditable actions better suited to a named command than a panel control.
- **A pinned, self-updating help message lives in the action items channel**: once a channel is configured, the bot posts and pins a message there explaining the transition emotes, who's allowed to use them, and how `/undo`/`/action-item` work. Any config change that affects its content (channel, emotes, approver role, approver user list) edits that same message in place rather than reposting, so it never drifts out of date. If the channel changes, editing the old message (now looked up against the new channel) naturally fails and falls back to posting+pinning a fresh message there — no special-casing needed.
- **Existing test data**: since only manual test data exists in the current deployment, the migration adding `guild_id NOT NULL` does not need a backfill path — it assumes the `action_items` table is truncated before deploying this change. Flag this to the user before running migrations for real.

## Data model changes

New migration `0002_multi_tenant` (up/down):

**`guild_configs`** (one row per guild, lazily created):
| column | type | notes |
|---|---|---|
| guild_id | text (pk) | |
| action_items_channel_id | text, default `''` | empty = not configured yet |
| approver_role_id | text, default `''` | empty = no role configured |
| in_progress_emote | text, default `'🔄'` | |
| done_emote | text, default `'✅'` | |
| help_message_id | text, default `''` | id of the pinned explainer message in `action_items_channel_id`; empty = not posted yet |

**`approvers`** (per-guild individually-added approvers):
| column | type | notes |
|---|---|---|
| guild_id | text | |
| user_id | text | |
| primary key | (guild_id, user_id) | |

**`action_items`** alterations:
- add `guild_id text not null` (no default — requires empty table at migration time)
- add `previous_status text not null default ''` — the state to restore to on `/undo`, set only when transitioning to `done`
- `status` values change from `pending`/`completed` to `new`/`in_progress`/`done` (app-level only, column stays `text`)
- replace `idx_action_items_message_id ... WHERE status = 'pending'` with `WHERE status <> 'done'`
- replace `idx_action_items_completed_at ... WHERE status = 'completed'` with `WHERE status = 'done'`
- add `idx_action_items_guild_id ON action_items (guild_id)`

## Configuration (environment variables — much smaller now)

- `DISCORD_TOKEN`
- `DATABASE_URL`
- `HEALTH_PORT` (unchanged, default `8080`)

Removed: `GUILD_ID`, `ACTION_ITEMS_CHANNEL_ID`, `APPROVER_USER_IDS`, `APPROVER_ROLE_ID` — all now per-guild, database-backed, managed via commands.

## Commands

- **`/action-item text:"..."`** — looks up the invoking guild's `action_items_channel_id`; if unset, responds ephemerally asking an approver/owner to run `/config` first. Posts with a `[NEW] ` prefix.
- **`/undo`** — ephemeral select menu, last 5 done items within 24h, scoped by `guild_id`, gated by owner-or-approver.
- **`/approver add user:@X`** / **`/approver remove user:@X`** / **`/approver list`** — subcommands under `/approver`. Gated by owner-or-approver.
- **`/config`** — single command, no options, gated by owner-or-approver. Opens an ephemeral panel: a summary line, a Channel Select (auto-saves), a Role Select (auto-saves, clearable), and an "Edit Emotes" button opening a modal with two pre-filled text inputs. Every component/modal interaction re-checks owner-or-approver.

## Permission check

`isOwnerOrApprover(s, guildID, member)`: fetch the guild (state cache, REST fallback) and compare `guild.OwnerID` to the member's user ID; if not a match, delegate to the per-guild approver check (role match against `guild_configs.approver_role_id`, or membership in `approvers`).

## State machine & reaction flows

- **Reaction add**: look up the pending item by `message_id` (works for both `new` and `in_progress`, both keep a live message). Determine which configured emote was reacted; ignore unrecognized emotes. Check owner-or-approver. In-progress emote on a `new` item → transition to `in_progress`, edit message prefix. Done emote from `new` or `in_progress` → record `previous_status`, delete the message, set `status = done`.
- **Reaction remove**: only meaningful for the in-progress emote on an `in_progress` item. Check owner-or-approver. Transition back to `new`, edit message prefix.
- **`/undo` selection**: restore `status` to the item's `previous_status`, repost with the matching prefix, update `message_id`, clear completion fields.

## Pinned help message

Built from the current `guild_configs` row plus its approvers: the configured emotes and what they do, who's allowed to use them (owner, approver role, approver users), how `/action-item`/`/undo` work, and that `/config`/`/approver` change these settings. Posted+pinned once a channel is configured; edited in place on every config-mutating action afterward.
