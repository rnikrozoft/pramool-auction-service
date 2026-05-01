package repository

import (
	"context"

	"github.com/uptrace/bun"
)

func (r auctionRepo) LockAuctionForSettlement(ctx context.Context, tx bun.Tx, auctionID string) (AuctionSettlementLock, error) {
	var row AuctionSettlementLock
	err := tx.QueryRowContext(ctx, `
		SELECT seller_id, status, end_at
		FROM auctions
		WHERE auction_id = ?
		FOR UPDATE
	`, auctionID).Scan(&row.SellerID, &row.Status, &row.EndAt)
	return row, err
}

func (r auctionRepo) SelectWinningBidHold(ctx context.Context, tx bun.Tx, auctionID string) (userID string, heldAmount int64, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, held_amount
		FROM auction_bid_holds
		WHERE auction_id = ? AND hold_status = 'held'
		ORDER BY held_amount DESC, created_at ASC
		LIMIT 1
	`, auctionID).Scan(&userID, &heldAmount)
	return userID, heldAmount, err
}

func (r auctionRepo) CloseAuctionNoWinner(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET status = 'closed', settled_at = NOW(), updated_at = NOW()
		WHERE auction_id = ?
	`, auctionID)
	return err
}

func (r auctionRepo) LockUserCredit(ctx context.Context, tx bun.Tx, userID string) (credit int64, err error) {
	err = tx.QueryRowContext(ctx, `SELECT credit FROM users WHERE user_id = ? FOR UPDATE`, userID).Scan(&credit)
	return credit, err
}

func (r auctionRepo) SetUserCredit(ctx context.Context, tx bun.Tx, userID string, credit int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE users SET credit = ?, updated_at = NOW() WHERE user_id = ?`, credit, userID)
	return err
}

func (r auctionRepo) InsertSellerEarningSettled(ctx context.Context, tx bun.Tx, sellerID, auctionID, winnerUserID string, amount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO seller_earnings (seller_id, auction_id, winner_user_id, amount, status)
		VALUES (?, ?, ?, ?, 'settled')
		ON CONFLICT (auction_id) DO NOTHING
	`, sellerID, auctionID, winnerUserID, amount)
	return err
}

func (r auctionRepo) SelectLosingBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) ([]LosingBidHold, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, held_amount
		FROM auction_bid_holds
		WHERE auction_id = ? AND hold_status = 'held' AND user_id <> ?
	`, auctionID, winnerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LosingBidHold
	for rows.Next() {
		var h LosingBidHold
		if err := rows.Scan(&h.UserID, &h.HeldAmount); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r auctionRepo) InsertBidRefundTransaction(ctx context.Context, tx bun.Tx, userID, auctionID string, amount, balanceBefore, balanceAfter int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note)
		VALUES (?, ?, 'bid_refund', ?, ?, ?, 'refund after auction settlement')
	`, userID, auctionID, amount, balanceBefore, balanceAfter)
	return err
}

func (r auctionRepo) ReleaseNonWinningBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auction_bid_holds
		SET hold_status = 'released', released_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND hold_status = 'held' AND user_id <> ?
	`, auctionID, winnerUserID)
	return err
}

func (r auctionRepo) SettleWinningBidHold(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auction_bid_holds
		SET hold_status = 'settled', settled_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND user_id = ?
	`, auctionID, winnerUserID)
	return err
}

func (r auctionRepo) CloseAuctionWithWinner(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET status = 'closed', winner_id = ?, settled_at = NOW(), updated_at = NOW()
		WHERE auction_id = ?
	`, winnerUserID, auctionID)
	return err
}
