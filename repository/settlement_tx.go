package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

func (r auctionRepo) LockAuctionRowForUpdate(ctx context.Context, tx bun.Tx, auctionID string) (*entity.Auction, error) {
	item := new(entity.Auction)
	err := tx.QueryRowContext(ctx, `
		SELECT auction_id, seller_id, title, category, item_condition, description,
			start_price, bid_step, current_bid, total_bids, status, end_at,
			COALESCE(allow_early_close, FALSE),
			COALESCE(early_close_hold_amount, 0),
			COALESCE(buy_now_price, 0),
			cover_image_url,
			COALESCE(winner_id, ''),
			seller_shipped_at, buyer_received_at, seller_payout_at,
			COALESCE(payout_early_close, FALSE),
			created_at, updated_at
		FROM auctions
		WHERE auction_id = ?
		FOR UPDATE
	`, auctionID).Scan(
		&item.AuctionID, &item.SellerID, &item.Title, &item.Category, &item.Condition,
		&item.Description, &item.StartPrice, &item.BidStep, &item.CurrentBid, &item.TotalBids,
		&item.Status, &item.EndAt, &item.AllowEarlyClose, &item.EarlyCloseHoldAmount,
		&item.BuyNowPrice,
		&item.CoverImageURL, &item.WinnerID, &item.SellerShippedAt, &item.BuyerReceivedAt, &item.SellerPayoutAt,
		&item.PayoutEarlyClose, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r auctionRepo) SealAuctionBiddingEndNow(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions SET end_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND status = 'active'
	`, auctionID)
	return err
}

func (r auctionRepo) LockAuctionForSettlement(ctx context.Context, tx bun.Tx, auctionID string) (AuctionSettlementLock, error) {
	var row AuctionSettlementLock
	err := tx.QueryRowContext(ctx, `
		SELECT seller_id, status, end_at, current_bid, start_price,
			COALESCE(allow_early_close, FALSE),
			COALESCE(early_close_hold_amount, 0)
		FROM auctions
		WHERE auction_id = ?
		FOR UPDATE
	`, auctionID).Scan(&row.SellerID, &row.Status, &row.EndAt, &row.CurrentBid, &row.StartPrice, &row.AllowEarlyClose, &row.EarlyCloseHoldAmount)
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

func (r auctionRepo) ZeroEarlyCloseHold(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions SET early_close_hold_amount = 0, updated_at = NOW() WHERE auction_id = ?
	`, auctionID)
	return err
}

func (r auctionRepo) RefundEarlyCloseHold(ctx context.Context, tx bun.Tx, sellerID, auctionID string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	before, err := r.LockUserCredit(ctx, tx, sellerID)
	if err != nil {
		return err
	}
	after := before + amount
	if err := r.SetUserCredit(ctx, tx, sellerID, after); err != nil {
		return err
	}
	if err := r.InsertCreditLedgerTransaction(ctx, tx, sellerID, auctionID, "early_close_hold_refund", amount, before, after, "คืนมัดจำปิดประมูลก่อนเวลา"); err != nil {
		return err
	}
	return r.ZeroEarlyCloseHold(ctx, tx, auctionID)
}

func (r auctionRepo) CloseAuctionNoWinner(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET status = 'closed', end_at = NOW(), settled_at = NOW(), updated_at = NOW()
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
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'bid_refund', ?, ?, ?, 'refund after auction settlement', ?)
	`, userID, auctionID, amount, balanceBefore, balanceAfter, amount)
	return err
}

func (r auctionRepo) InsertCreditLedgerTransaction(ctx context.Context, tx bun.Tx, userID, auctionID, txType string, amount, balanceBefore, balanceAfter int64, note string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
	`, userID, auctionID, txType, amount, balanceBefore, balanceAfter, note)
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
		WHERE auction_id = ? AND user_id = ? AND hold_status = 'escrow'
	`, auctionID, winnerUserID)
	return err
}

func (r auctionRepo) CloseAuctionWithWinner(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string, payoutEarlyClose bool) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET status = 'closed', winner_id = ?, end_at = NOW(), settled_at = NOW(), updated_at = NOW(),
		    payout_early_close = ?
		WHERE auction_id = ?
	`, winnerUserID, payoutEarlyClose, auctionID)
	return err
}

func (r auctionRepo) MoveWinningHoldToEscrow(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE auction_bid_holds
		SET hold_status = 'escrow', updated_at = NOW()
		WHERE auction_id = ? AND user_id = ? AND hold_status = 'held'
	`, auctionID, winnerUserID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expected 1 winning hold moved to escrow, got %d", n)
	}
	return nil
}

func (r auctionRepo) LockAuctionForEscrowRelease(ctx context.Context, tx bun.Tx, auctionID string) (EscrowReleaseLock, error) {
	var row EscrowReleaseLock
	var shippedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT seller_id, COALESCE(winner_id, ''), start_price, COALESCE(payout_early_close, FALSE),
			(seller_shipped_at IS NOT NULL), (seller_payout_at IS NOT NULL), seller_shipped_at
		FROM auctions
		WHERE auction_id = ?
		FOR UPDATE
	`, auctionID).Scan(&row.SellerID, &row.WinnerID, &row.StartPrice, &row.PayoutEarlyClose, &row.SellerShipped, &row.PayoutDone, &shippedAt)
	if err != nil {
		return row, err
	}
	if shippedAt.Valid {
		t := shippedAt.Time
		row.SellerShippedAt = &t
	}
	return row, nil
}

func (r auctionRepo) GetWinnerEscrowHoldAmount(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) (int64, error) {
	var amt int64
	err := tx.QueryRowContext(ctx, `
		SELECT held_amount
		FROM auction_bid_holds
		WHERE auction_id = ? AND user_id = ? AND hold_status = 'escrow'
	`, auctionID, winnerUserID).Scan(&amt)
	return amt, err
}

func (r auctionRepo) MarkAuctionDeliveryCompleted(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET buyer_received_at = NOW(), seller_payout_at = NOW(), updated_at = NOW()
		WHERE auction_id = ?
	`, auctionID)
	return err
}

func (r auctionRepo) MarkSellerShipped(ctx context.Context, auctionID, sellerID string) (int64, error) {
	res, err := r.bun.ExecContext(ctx, `
		UPDATE auctions
		SET seller_shipped_at = NOW(), updated_at = NOW()
		WHERE auction_id = ?
		  AND seller_id = ?
		  AND status = 'closed'
		  AND COALESCE(NULLIF(TRIM(winner_id), ''), '') <> ''
		  AND seller_shipped_at IS NULL
		  AND seller_payout_at IS NULL
	`, auctionID, sellerID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
