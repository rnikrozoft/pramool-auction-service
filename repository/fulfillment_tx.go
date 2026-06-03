package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

const SellerNoShipPenaltyPoints = -5

// BuyerConfirmReviewRewardPoints is added to buyer reputation when they confirm receipt with a review.
const BuyerConfirmReviewRewardPoints = 1

// NoShipRefundLock is an auction row eligible for buyer escrow refund after seller missed ship deadline.
type NoShipRefundLock struct {
	SellerID  string
	WinnerID  string
	SettledAt time.Time
}

func (r auctionRepo) LockAuctionForNoShipRefund(ctx context.Context, tx bun.Tx, auctionID string, afterSettledDays int) (NoShipRefundLock, error) {
	var row NoShipRefundLock
	err := tx.QueryRowContext(ctx, `
		SELECT seller_id, COALESCE(winner_id::text, ''), settled_at
		FROM auctions
		WHERE auction_id = ?
		  AND status = 'closed'
		  AND winner_id IS NOT NULL
		  AND seller_shipped_at IS NULL
		  AND seller_payout_at IS NULL
		  AND buyer_escrow_refunded_at IS NULL
		  AND settled_at IS NOT NULL
		  AND (
		    (admin_ship_deadline_at IS NOT NULL AND admin_ship_deadline_at <= NOW())
		    OR (admin_ship_deadline_at IS NULL AND settled_at <= NOW() - (? * INTERVAL '1 day'))
		  )
		FOR UPDATE
	`, auctionID, afterSettledDays).Scan(&row.SellerID, &row.WinnerID, &row.SettledAt)
	return row, err
}

func (r auctionRepo) MarkBuyerEscrowRefunded(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET buyer_escrow_refunded_at = NOW(), updated_at = NOW()
		WHERE auction_id = ?
	`, auctionID)
	return err
}

func (r auctionRepo) MarkSellerPayoutOnly(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET seller_payout_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND seller_payout_at IS NULL
	`, auctionID)
	return err
}

func (r auctionRepo) MarkBuyerReceivedOnly(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auctions
		SET buyer_received_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND buyer_received_at IS NULL
	`, auctionID)
	return err
}

func (r auctionRepo) ReleaseEscrowHoldAsRefunded(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE auction_bid_holds
		SET hold_status = 'released', released_at = NOW(), updated_at = NOW()
		WHERE auction_id = ? AND user_id = ? AND hold_status = 'escrow'
	`, auctionID, winnerUserID)
	return err
}

func (r auctionRepo) AdjustReputationPoints(ctx context.Context, tx bun.Tx, userID string, delta int) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET reputation_points = reputation_points + ?,
		    updated_at = NOW()
		WHERE user_id = ?
	`, delta, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r auctionRepo) IncrementSellerNoShipCount(ctx context.Context, tx bun.Tx, sellerID string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET seller_no_ship_count = seller_no_ship_count + 1,
		    updated_at = NOW()
		WHERE user_id = ?
	`, sellerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r auctionRepo) ListFulfillmentSweepAuctionIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.bun.QueryContext(ctx, `
		SELECT auction_id
		FROM auctions
		WHERE status = 'closed'
		  AND winner_id IS NOT NULL
		  AND buyer_received_at IS NULL
		  AND buyer_escrow_refunded_at IS NULL
		  AND (
		    (seller_shipped_at IS NULL AND seller_payout_at IS NULL AND settled_at IS NOT NULL)
		    OR (seller_shipped_at IS NOT NULL AND seller_payout_at IS NULL AND LOWER(COALESCE(shipment_status, 'pending')) = 'delivered')
		  )
		ORDER BY updated_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r auctionRepo) ListUserFulfillmentAuctionIDs(ctx context.Context, userID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	rows, err := r.bun.QueryContext(ctx, `
		SELECT auction_id
		FROM auctions
		WHERE status = 'closed'
		  AND (seller_id = $1 OR winner_id = $1)
		  AND buyer_received_at IS NULL
		  AND buyer_escrow_refunded_at IS NULL
		  AND winner_id IS NOT NULL
		ORDER BY updated_at ASC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
