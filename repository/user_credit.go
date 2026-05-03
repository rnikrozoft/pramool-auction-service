package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// UserCreditRepository updates users.credit within an existing transaction (listing deposit).
type UserCreditRepository interface {
	DeductListingDepositTx(ctx context.Context, tx bun.Tx, userID string, amount int64) (ok bool, balanceBefore, balanceAfter int64, err error)
}

type userCreditRepo struct{}

// NewUserCreditRepository returns a repository that mutates users.credit via the passed *bun.DB only for constructor symmetry; Deduct uses the provided tx.
func NewUserCreditRepository(_ *bun.DB) UserCreditRepository {
	return userCreditRepo{}
}

func (r userCreditRepo) DeductListingDepositTx(ctx context.Context, tx bun.Tx, userID string, amount int64) (bool, int64, int64, error) {
	if amount <= 0 {
		return false, 0, 0, fmt.Errorf("invalid deduct amount")
	}
	var after, before int64
	err := tx.NewRaw(`
		UPDATE users
		SET credit = credit - ?, updated_at = NOW()
		WHERE user_id = ? AND COALESCE(credit, 0) >= ?
		RETURNING COALESCE(credit, 0), COALESCE(credit, 0) + ?
	`, amount, userID, amount, amount).Scan(ctx, &after, &before)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, 0, nil
	}
	if err != nil {
		return false, 0, 0, err
	}
	return true, before, after, nil
}
