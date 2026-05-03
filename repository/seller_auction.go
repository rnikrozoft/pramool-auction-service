package repository

import (
	"context"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

func (r auctionRepo) CreateAuctionWithTx(ctx context.Context, tx bun.Tx, auction entity.Auction) error {
	query := `
	INSERT INTO auctions (
		auction_id, seller_id, title, category, item_condition, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, allow_early_close, early_close_hold_amount, buy_now_price, cover_image_url
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := tx.NewRaw(query,
		auction.AuctionID,
		auction.SellerID,
		auction.Title,
		auction.Category,
		auction.Condition,
		auction.Description,
		auction.StartPrice,
		auction.BidStep,
		auction.CurrentBid,
		auction.TotalBids,
		auction.Status,
		auction.EndAt,
		auction.AllowEarlyClose,
		auction.EarlyCloseHoldAmount,
		auction.BuyNowPrice,
		auction.CoverImageURL,
	).Exec(ctx)
	return err
}

func (r auctionRepo) InsertListingDepositHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64, note string) error {
	if holdAmount <= 0 {
		return nil
	}
	ledgerDelta := -holdAmount
	query := `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'listing_deposit_hold', ?, ?, ?, ?, ?)
	`
	_, err := tx.NewRaw(query, sellerID, auctionID, ledgerDelta, balanceBefore, balanceAfter, note, holdAmount).Exec(ctx)
	return err
}

func (r auctionRepo) CreateAuctionImagesWithTx(ctx context.Context, tx bun.Tx, images []entity.AuctionImage) error {
	query := `INSERT INTO auction_images (auction_id, image_url, sort_order) VALUES (?, ?, ?)`
	for _, img := range images {
		if _, err := tx.NewRaw(query, img.AuctionID, img.ImageURL, img.SortOrder).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r auctionRepo) ListAuctionsBySellerID(ctx context.Context, sellerID string) ([]entity.Auction, error) {
	items := make([]entity.Auction, 0)
	query := `
	SELECT auction_id, seller_id, title, category, item_condition AS condition, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, COALESCE(allow_early_close, FALSE) AS allow_early_close,
		COALESCE(buy_now_price, 0) AS buy_now_price, cover_image_url,
		created_at, updated_at
	FROM auctions
	WHERE seller_id = ?
	ORDER BY created_at DESC
	`
	err := r.bun.NewRaw(query, sellerID).Scan(ctx, &items)
	return items, err
}

func (r auctionRepo) LockAuctionBySellerForUpdate(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (*entity.Auction, error) {
	item := new(entity.Auction)
	err := tx.NewRaw(`
		SELECT auction_id, seller_id, title, category, item_condition AS condition, description,
			start_price, bid_step, current_bid, total_bids, status, end_at,
			COALESCE(allow_early_close, FALSE) AS allow_early_close,
			COALESCE(early_close_hold_amount, 0) AS early_close_hold_amount,
			cover_image_url,
			created_at, updated_at, COALESCE(winner_id, '') AS winner_id
		FROM auctions
		WHERE auction_id = ? AND seller_id = ?
		FOR UPDATE
	`, auctionID, sellerID).Scan(ctx, item)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r auctionRepo) CountAuctionBidsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error) {
	var n int64
	err := tx.NewRaw(`SELECT COUNT(*)::bigint FROM auction_bids WHERE auction_id = ?`, auctionID).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) CountHeldBidHoldsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error) {
	var n int64
	err := tx.NewRaw(`SELECT COUNT(*)::bigint FROM auction_bid_holds WHERE auction_id = ? AND hold_status = 'held'`, auctionID).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) DeleteClosedAuctionNoBidsTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (int64, error) {
	res, err := tx.NewRaw(`
		DELETE FROM auctions
		WHERE auction_id = ?
		  AND seller_id = ?
		  AND status = 'closed'
		  AND total_bids = 0
		  AND (winner_id IS NULL OR TRIM(winner_id) = '')
	`, auctionID, sellerID).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r auctionRepo) ApplyAuctionReopenTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string, endAt time.Time) (int64, error) {
	res, err := tx.NewRaw(`
		UPDATE auctions SET
			status = 'active',
			current_bid = start_price,
			total_bids = 0,
			end_at = ?,
			winner_id = NULL,
			settled_at = NULL,
			early_close_hold_amount = 0,
			updated_at = NOW()
		WHERE auction_id = ? AND seller_id = ?
		  AND status = 'closed'
		  AND total_bids = 0
		  AND (winner_id IS NULL OR winner_id = '')
	`, endAt, auctionID, sellerID).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
