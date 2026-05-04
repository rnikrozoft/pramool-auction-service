package repository

import (
	"context"
	"strings"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

func sellerAuctionScopeSQL(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "active":
		return "AND status = 'active' AND end_at > NOW()"
	case "closed":
		return "AND NOT (status = 'active' AND end_at > NOW())"
	default:
		return ""
	}
}

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

func (r auctionRepo) ListAuctionsBySellerID(ctx context.Context, sellerID, scope string, limit, offset int) ([]entity.Auction, error) {
	items := make([]entity.Auction, 0)
	extra := sellerAuctionScopeSQL(scope)
	query := `
	SELECT auction_id, seller_id, title, category, item_condition AS condition, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, COALESCE(allow_early_close, FALSE) AS allow_early_close,
		COALESCE(buy_now_price, 0) AS buy_now_price, cover_image_url,
		COALESCE(NULLIF(TRIM(BOTH FROM COALESCE(winner_id::text, '')), ''), '') AS winner_id,
		seller_shipped_at,
		seller_payout_at,
		seller_close_pause_bids_until,
		created_at, updated_at
	FROM auctions
	WHERE seller_id = ?
	` + extra + `
	ORDER BY created_at DESC, auction_id DESC
	LIMIT ? OFFSET ?
	`
	err := r.bun.NewRaw(query, sellerID, limit, offset).Scan(ctx, &items)
	return items, err
}

func (r auctionRepo) CountAuctionsBySellerID(ctx context.Context, sellerID string) (int, error) {
	var n int
	err := r.bun.NewRaw(`SELECT COUNT(*)::int FROM auctions WHERE seller_id = ?`, sellerID).Scan(ctx, &n)
	return n, err
}

// CountSellerAuctionsDisplayActive matches the seller table UI: status active and end_at still in the future.
func (r auctionRepo) CountSellerAuctionsDisplayActive(ctx context.Context, sellerID string) (int, error) {
	var n int
	err := r.bun.NewRaw(`
		SELECT COUNT(*)::int FROM auctions
		WHERE seller_id = ? AND status = 'active' AND end_at > NOW()
	`, sellerID).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) CountAuctionsBySellerIDScoped(ctx context.Context, sellerID, scope string) (int, error) {
	extra := sellerAuctionScopeSQL(scope)
	query := `SELECT COUNT(*)::int FROM auctions WHERE seller_id = ? ` + extra
	var n int
	err := r.bun.NewRaw(query, sellerID).Scan(ctx, &n)
	return n, err
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
			seller_close_pause_bids_until = NULL,
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
