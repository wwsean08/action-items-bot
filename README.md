# action-items-bot

A Discord bot for tracking action items in a text channel. Items are created with a slash
command, transitioned with emoji reactions, and stored per-guild in Postgres.

## How it works

- **Create**: `/action-item text:"..."` posts a new item to the guild's configured channel.
- **Transition**: react to an item with the "in progress" emote (default 🔄) to mark it in
  progress, or the "done" emote (default ✅) to complete it — completing removes the message.
  Removing the in-progress reaction moves it back to new.
- **Undo**: `/undo` lists the 5 most recently completed items (within 24 hours) and lets an
  approver restore one.
- **Search**: `/search query:"..."` looks through completed items; results are shown only to
  the requester.
- **Configure**: `/config` opens a panel to set the action items channel, the in-progress/done
  emotes, and the approver role. `/approver add|remove|list` manages individual approver users.

Who can transition/undo/configure items is governed by `/approver` and the configured approver
role, plus the guild owner (always allowed). Creating and searching items follow the guild's
Discord "Integrations" permissions for those commands, like any other slash command.

## Requirements

- Go (version pinned in `go.mod`)
- Postgres (for local development, `docker compose up -d postgres` starts one)
- A Discord bot application/token with the following privileged gateway intents enabled:
  `Guilds`, `Guild Messages`, `Guild Message Reactions`, `Guild Members`

## Configuration

The bot is configured entirely through environment variables (see `.env.example`):

| Variable        | Required | Description                                  |
|-----------------|----------|-----------------------------------------------|
| `DISCORD_TOKEN` | yes      | Discord bot token                             |
| `DATABASE_URL`  | yes      | Postgres connection string                    |
| `HEALTH_PORT`   | no       | Port for the health check server (default `8080`) |

## Running locally

```bash
docker compose up -d postgres   # start Postgres
export DISCORD_TOKEN=...
export DATABASE_URL=postgres://action_items:action_items@localhost:5432/action_items?sslmode=disable
go run ./cmd/bot
```

Or run the whole stack (bot + Postgres) with Docker Compose:

```bash
export DISCORD_TOKEN=...
docker compose up --build
```

On startup the bot runs pending database migrations automatically, connects to Discord, and
registers its slash commands globally (they may take up to an hour to appear in a new guild the
first time). A health check endpoint is exposed at `:$HEALTH_PORT`.

## Development

```bash
go build ./...                                 # build
go vet ./...                                   # vet
go test -race ./...                            # unit tests
go test -race -tags=integration ./...          # integration tests (needs Postgres)
golangci-lint run                              # lint (matches CI)
```

See `CLAUDE.md` for a deeper architecture overview.
