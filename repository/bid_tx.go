package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

func (r auctionRepo) SelectBidHoldForUpdate(ctx context.Context, tx bun.Tx, auctionID, bidderID string) (oldHeldAmount int64, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(held_amount, 0)
		FROM auction_bid_holds
		WHERE auction_id = ? AND user_id = ?
		FOR UPDATE
	`, auctionID, bidderID).Scan(&oldHeldAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return oldHeldAmount, err
}

func (r auctionRepo) UpdateAuctionOnBid(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) (*entity.Auction, error) {
	item := new(entity.Auction)
	err := tx.QueryRowContext(ctx, `
		UPDATE auctions
		SET current_bid = ?,
			total_bids = total_bids + 1,
			updated_at = NOW()
		WHERE auction_id = ?
		  AND seller_id <> ?
		  AND status = 'active'
		  AND end_at > NOW()
		  AND ? >= current_bid + bid_step
		RETURNING auction_id, seller_id, title, category, item_condition, description,
		          start_price, current_bid, bid_step, total_bids, status, end_at, cover_image_url
	`, amount, auctionID, bidderID, amount).Scan(
		&item.AuctionID, &item.SellerID, &item.Title, &item.Category, &item.Condition,
		&item.Description, &item.StartPrice, &item.CurrentBid, &item.BidStep, &item.TotalBids,
		&item.Status, &item.EndAt, &item.CoverImageURL,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBidConflict
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r auctionRepo) InsertAuctionBidRecord(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_bids (auction_id, bidder_user_id, bid_amount)
		VALUES (?, ?, ?)
	`, auctionID, bidderID, amount)
	return err
}

func (r auctionRepo) UpsertAuctionBidHold(ctx context.Context, tx bun.Tx, auctionID, bidderID string, heldAmount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_bid_holds (auction_id, user_id, held_amount, hold_status, updated_at)
		VALUES (?, ?, ?, 'held', NOW())
		ON CONFLICT (auction_id, user_id)
		DO UPDATE SET held_amount = EXCLUDED.held_amount, hold_status = 'held', updated_at = NOW()
	`, auctionID, bidderID, heldAmount)
	return err
}

func (r auctionRepo) InsertBidHoldAdjustmentTransaction(ctx context.Context, tx bun.Tx, bidderID, auctionID string, delta, balanceBefore, balanceAfter, bidAmount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'bid_hold', ?, ?, ?, 'hold amount adjusted by new bid', ?)
	`, bidderID, auctionID, delta, balanceBefore, balanceAfter, bidAmount)
	return err
}
