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

func scanItem(row scanner) (actionitems.ActionItem, error) {
	var item actionitems.ActionItem
	var status string
	err := row.Scan(
		&item.ID,
		&item.Description,
		&item.CreatedByUserID,
		&item.CreatedAt,
		&item.MessageID,
		&status,
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
	return item, nil
}

const selectColumns = `id, description, created_by_user_id, created_at, message_id, status, completed_by_user_id, completed_at`

func (r *Repository) Create(ctx context.Context, item actionitems.ActionItem) (actionitems.ActionItem, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO action_items (description, created_by_user_id, created_at, message_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+selectColumns,
		item.Description, item.CreatedByUserID, item.CreatedAt, item.MessageID, string(item.Status),
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
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM action_items WHERE message_id = $1 AND status = $2 LIMIT 1`,
		messageID, string(actionitems.StatusPending))
	return scanItem(row)
}

func (r *Repository) Complete(ctx context.Context, id, completedByUserID string, completedAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE action_items SET status = $1, completed_by_user_id = $2, completed_at = $3 WHERE id = $4`,
		string(actionitems.StatusCompleted), completedByUserID, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("completing action item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}

func (r *Repository) ListCompletedSince(ctx context.Context, since time.Time, limit int) ([]actionitems.ActionItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+selectColumns+` FROM action_items
		WHERE status = $1 AND completed_at >= $2
		ORDER BY completed_at DESC
		LIMIT $3`,
		string(actionitems.StatusCompleted), since, limit,
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

func (r *Repository) Reopen(ctx context.Context, id, newMessageID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE action_items
		SET status = $1, message_id = $2, completed_by_user_id = '', completed_at = NULL
		WHERE id = $3`,
		string(actionitems.StatusPending), newMessageID, id,
	)
	if err != nil {
		return fmt.Errorf("reopening action item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return actionitems.ErrNotFound
	}
	return nil
}
