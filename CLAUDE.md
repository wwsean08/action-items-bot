# action-items-bot

Discord bot that tracks action items via slash commands and message reactions, backed by Postgres.

## Commands

```bash
go build ./...                                              # build
go vet ./...                                                 # vet
go test -race ./...                                          # unit tests
go test -race -tags=integration ./...                        # integration tests (needs Postgres)
docker compose up -d postgres                                 # start local Postgres for integration tests
golangci-lint run                                              # lint (matches CI)
```

Integration tests live in `internal/store/postgres/repository_integration_test.go` behind the `integration` build tag and read `TEST_DATABASE_URL` (defaults to the docker-compose Postgres on `localhost:5432`).

## Architecture

- `cmd/bot/main.go` — wiring: loads config, runs migrations, opens the Discord session, starts the health server.
- `internal/config` — env-based config (`DISCORD_TOKEN`, `DATABASE_URL`, `HEALTH_PORT`).
- `internal/actionitems` — domain layer: `Service`, `ActionItem` types, status state machine (`new` → `in_progress` → `done`), approver logic.
- `internal/discord` — Discord-facing layer: slash command handlers (`bot.go`, `commands.go`), reaction handlers (`reactions.go`), permissions (`permissions.go`), config panel, help text.
- `internal/store/postgres` — Postgres repository implementing the `actionitems.Repository` interface; migrations in `internal/store/postgres/migrations` (golang-migrate, numbered `NNNN_name.up/down.sql`).
- `internal/health` — HTTP health check server exposed on `HEALTH_PORT` (default 8080).

Status changes flow through Discord message reactions (🔄 in-progress, ✅ done — see `actionitems.DefaultInProgressEmote`/`DefaultDoneEmote`) as well as slash commands.

## Gotchas

- Guild-scoped multi-tenancy was added in migration `0002_multi_tenant.sql` — action items are keyed by `GuildID`; see `docs/superpowers/specs/2026-08-24-multi-tenant-state-machine-design.md` for the design rationale.
- Integration tests require the `integration` build tag AND a running Postgres — they're excluded from plain `go test ./...`.
- CI (`golangci-lint`) only reports *new* issues on PRs, so pre-existing lint debt won't block a PR.
- Real config comes from environment variables (`DISCORD_TOKEN`, `DATABASE_URL`, `HEALTH_PORT`); `.env.example` documents the shape. `.env*` files are gitignored (except `.env.example`) and Claude Code's safety hook blocks reading them directly — ask the user for values instead.
