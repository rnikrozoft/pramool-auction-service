package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/uptrace/bun"
)

type UserSuspensionRepository struct {
	db *bun.DB
}

func NewUserSuspensionRepository(db *bun.DB) *UserSuspensionRepository {
	return &UserSuspensionRepository{db: db}
}

func (r *UserSuspensionRepository) IsUserBanned(ctx context.Context, subject string) (bool, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return false, nil
	}
	var banned bool
	err := r.db.NewRaw(`
		SELECT (
			suspended_at IS NOT NULL
			OR (restricted_until IS NOT NULL AND restricted_until > NOW())
		)
		FROM users
		WHERE user_id = ? OR tel = ?
		LIMIT 1
	`, subject, subject).Scan(ctx, &banned)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return banned, err
}

func (r *UserSuspensionRepository) IsUserPostingBanned(ctx context.Context, subject string) (bool, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return false, nil
	}
	var banned bool
	err := r.db.NewRaw(`
		SELECT (
			suspended_at IS NOT NULL
			OR (restricted_until IS NOT NULL AND restricted_until > NOW())
			OR (posting_restricted_until IS NOT NULL AND posting_restricted_until > NOW())
		)
		FROM users
		WHERE user_id = ? OR tel = ?
		LIMIT 1
	`, subject, subject).Scan(ctx, &banned)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return banned, err
}
