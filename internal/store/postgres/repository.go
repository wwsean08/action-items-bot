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
