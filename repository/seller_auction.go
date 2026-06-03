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

func sellerAuctionSearchSQL(q string) (clause string, args []interface{}) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	pattern := "%" + q + "%"
	return ` AND (title ILIKE ? OR auction_id ILIKE ?)`, []interface{}{pattern, pattern}
}

func (r auctionRepo) CreateAuctionWithTx(ctx context.Context, tx bun.Tx, auction entity.Auction) error {
	query := `
	INSERT INTO auctions (
		auction_id, seller_id, title, category, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, allow_early_close, allow_bid_cancel, auto_renew, early_close_hold_amount, buy_now_price, cover_image_url
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := tx.NewRaw(query,
		auction.AuctionID,
		auction.SellerID,
		auction.Title,
		auction.Category,
		auction.Description,
		auction.StartPrice,
		auction.BidStep,
		auction.CurrentBid,
		auction.TotalBids,
		auction.Status,
		auction.EndAt,
		auction.AllowEarlyClose,
		auction.AllowBidCancel,
		auction.AutoRenew,
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

func (r auctionRepo) GetListingDepositHoldAmountTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error) {
	var amt int64
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(bid_amount), 0)::bigint
		FROM bid_transactions
		WHERE auction_id = ? AND tx_type = 'listing_deposit_hold'
	`, auctionID).Scan(&amt)
	return amt, err
}

func (r auctionRepo) InsertListingDepositForfeitTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, forfeitAmount int64, note string) error {
	if forfeitAmount <= 0 {
		return nil
	}
	query := `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'listing_deposit_forfeit', 0, 0, 0, ?, ?)
	`
	_, err := tx.NewRaw(query, sellerID, auctionID, note, forfeitAmount).Exec(ctx)
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

func (r auctionRepo) ListAuctionsBySellerID(ctx context.Context, sellerID, scope, q, sort, order string, limit, offset int) ([]entity.Auction, error) {
	items := make([]entity.Auction, 0)
	extra := sellerAuctionScopeSQL(scope)
	searchClause, searchArgs := sellerAuctionSearchSQL(q)
	orderBy := sellerAuctionSortSQL(sort, order)
	query := `
	SELECT auction_id, seller_id, title, category, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, COALESCE(allow_early_close, FALSE) AS allow_early_close,
		COALESCE(allow_bid_cancel, FALSE) AS allow_bid_cancel,
		COALESCE(buy_now_price, 0) AS buy_now_price, cover_image_url,
		COALESCE(NULLIF(TRIM(BOTH FROM COALESCE(winner_id::text, '')), ''), '') AS winner_id,
		seller_shipped_at,
		seller_payout_at,
		seller_close_pause_bids_until,
		created_at, updated_at,
		COALESCE(bid_stats.cnt, 0)::bigint AS bidder_count
	FROM auctions
	LEFT JOIN LATERAL (
		SELECT COUNT(*)::bigint AS cnt
		FROM (
			SELECT bidder_user_id FROM auction_bids WHERE auction_id = auctions.auction_id
			UNION
			SELECT bidder_user_id FROM auction_bid_participants WHERE auction_id = auctions.auction_id
		) u
	) bid_stats ON true
	WHERE seller_id = ?
	` + extra + searchClause + `
	ORDER BY ` + orderBy + `
	LIMIT ? OFFSET ?
	`
	args := append([]interface{}{sellerID}, searchArgs...)
	args = append(args, limit, offset)
	err := r.bun.NewRaw(query, args...).Scan(ctx, &items)
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

func (r auctionRepo) CountAuctionsBySellerIDScoped(ctx context.Context, sellerID, scope, q string) (int, error) {
	extra := sellerAuctionScopeSQL(scope)
	searchClause, searchArgs := sellerAuctionSearchSQL(q)
	query := `SELECT COUNT(*)::int FROM auctions WHERE seller_id = ? ` + extra + searchClause
	args := append([]interface{}{sellerID}, searchArgs...)
	var n int
	err := r.bun.NewRaw(query, args...).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) LockAuctionBySellerForUpdate(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (*entity.Auction, error) {
	item := new(entity.Auction)
	err := tx.NewRaw(`
		SELECT auction_id, seller_id, title, category, description,
			start_price, bid_step, current_bid, total_bids, status, end_at,
			COALESCE(allow_early_close, FALSE) AS allow_early_close,
			COALESCE(early_close_hold_amount, 0) AS early_close_hold_amount,
			cover_image_url,
			created_at, updated_at, COALESCE(winner_id::text, '') AS winner_id
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
		  AND winner_id IS NULL
	`, auctionID, sellerID).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r auctionRepo) ApplyAuctionAutoRenewTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string, endAt time.Time) (int64, error) {
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
			auto_renew_count = COALESCE(auto_renew_count, 0) + 1,
			updated_at = NOW()
		WHERE auction_id = ? AND seller_id = ?
		  AND status = 'active'
		  AND COALESCE(auto_renew, FALSE) = TRUE
	`, endAt, auctionID, sellerID).Exec(ctx)
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
			created_at = NOW(),
			updated_at = NOW()
		WHERE auction_id = ? AND seller_id = ?
		  AND status = 'closed'
		  AND total_bids = 0
		  AND winner_id IS NULL
	`, endAt, auctionID, sellerID).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
