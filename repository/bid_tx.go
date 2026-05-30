package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

func (r auctionRepo) UpdateAuctionOnBid(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64, newEndAt time.Time) (*entity.Auction, error) {
	item := new(entity.Auction)
	err := tx.QueryRowContext(ctx, `
		UPDATE auctions
		SET current_bid = ?,
			total_bids = total_bids + 1,
			end_at = ?,
			updated_at = NOW()
		WHERE auction_id = ?
		  AND seller_id <> ?
		  AND status = 'active'
		  AND end_at > NOW()
		  AND ? >= current_bid + bid_step
		RETURNING auction_id, seller_id, title, category, item_condition, description,
		          start_price, current_bid, bid_step, total_bids, status, end_at,
		          COALESCE(buy_now_price, 0), cover_image_url
	`, amount, newEndAt, auctionID, bidderID, amount).Scan(
		&item.AuctionID, &item.SellerID, &item.Title, &item.Category, &item.Condition,
		&item.Description, &item.StartPrice, &item.CurrentBid, &item.BidStep, &item.TotalBids,
		&item.Status, &item.EndAt, &item.BuyNowPrice, &item.CoverImageURL,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBidConflict
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

// UpsertAuctionBidLive keeps one row per bidder per auction while the listing is open (no per-click append).
func (r auctionRepo) UpsertAuctionBidLive(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_bids (auction_id, bidder_user_id, bid_amount, placed_at)
		VALUES (?, ?, ?, NOW())
		ON CONFLICT (auction_id, bidder_user_id)
		DO UPDATE SET
			bid_amount = EXCLUDED.bid_amount,
			placed_at = EXCLUDED.placed_at
		WHERE EXCLUDED.bid_amount >= auction_bids.bid_amount
	`, auctionID, bidderID, amount)
	return err
}

// UpsertAuctionBidParticipant records max bid per user for bid history after the auction closes.
func (r auctionRepo) UpsertAuctionBidParticipant(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_bid_participants (auction_id, bidder_user_id, max_bid_amount, last_bid_at)
		VALUES (?, ?, ?, NOW())
		ON CONFLICT (auction_id, bidder_user_id)
		DO UPDATE SET
			max_bid_amount = GREATEST(auction_bid_participants.max_bid_amount, EXCLUDED.max_bid_amount),
			last_bid_at = EXCLUDED.last_bid_at
	`, auctionID, bidderID, amount)
	return err
}

// ClearAuctionBidsLive removes live bidder rows when an auction settles (winner kept in auctions.winner_id + participants).
func (r auctionRepo) ClearAuctionBidsLive(ctx context.Context, tx bun.Tx, auctionID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM auction_bids WHERE auction_id = ?`, auctionID)
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
		VALUES (?, ?, 'bid_hold', ?, ?, ?, 'bid placed', ?)
	`, bidderID, auctionID, delta, balanceBefore, balanceAfter, bidAmount)
	return err
}
