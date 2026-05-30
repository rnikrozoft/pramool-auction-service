package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/uptrace/bun"
)

// UserDisplayName is first/last name from users for live bidder labels.
type UserDisplayName struct {
	FirstName string
	LastName  string
}

func (r auctionRepo) GetUserDisplayName(ctx context.Context, userID string) (UserDisplayName, error) {
	var row UserDisplayName
	err := r.bun.NewRaw(`
		SELECT COALESCE(first_name, ''), COALESCE(last_name, '')
		FROM users WHERE user_id = ?
	`, userID).Scan(ctx, &row.FirstName, &row.LastName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserDisplayName{}, nil
		}
		return UserDisplayName{}, err
	}
	return row, nil
}

type userDisplayNameRow struct {
	UserID    string `bun:"user_id"`
	FirstName string `bun:"first_name"`
	LastName  string `bun:"last_name"`
}

func (r auctionRepo) GetUserDisplayNamesByIDs(ctx context.Context, userIDs []string) (map[string]UserDisplayName, error) {
	out := make(map[string]UserDisplayName)
	seen := make(map[string]struct{}, len(userIDs))
	ids := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []userDisplayNameRow
	err := r.bun.NewRaw(`
		SELECT user_id, COALESCE(first_name, '') AS first_name, COALESCE(last_name, '') AS last_name
		FROM users
		WHERE user_id IN (?)
	`, bun.In(ids)).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = UserDisplayName{FirstName: row.FirstName, LastName: row.LastName}
	}
	return out, nil
}
