package repository

import (
	"context"
	"strings"
	"time"
)

// SellerReviewReceivedRow is one review on a seller's listing (public profile).
type SellerReviewReceivedRow struct {
	AuctionID    string    `bun:"auction_id"`
	AuctionTitle string    `bun:"auction_title"`
	Rating       float64   `bun:"rating"`
	CreatedAt    time.Time `bun:"created_at"`
}

func (r auctionRepo) GetPublicUserProfileRow(ctx context.Context, userID string) (SellerPublicProfile, time.Time, error) {
	var row SellerPublicProfile
	var createdAt time.Time
	err := r.bun.NewRaw(`
		SELECT COALESCE(first_name, ''), COALESCE(last_name, ''),
			COALESCE(seller_review_points_total, 0), COALESCE(seller_review_count, 0),
			created_at
		FROM users
		WHERE user_id = ?
	`, userID).Scan(ctx, &row.FirstName, &row.LastName, &row.ReviewPointsTotal, &row.ReviewCount, &createdAt)
	return row, createdAt, err
}

func (r auctionRepo) ListSellerActiveAuctionsPublic(ctx context.Context, sellerID string, limit, offset int) ([]PublicAuctionRow, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	sellerID = strings.TrimSpace(sellerID)
	if sellerID == "" {
		return nil, nil
	}
	var rows []PublicAuctionRow
	err := r.bun.NewRaw(`
		SELECT
			a.auction_id,
			a.title,
			a.category,
			a.start_price,
			a.current_bid,
			a.bid_step,
			a.total_bids,
			a.end_at,
			a.cover_image_url,
			COALESCE(a.buy_now_price, 0)::bigint AS buy_now_price,
			COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
			a.created_at,
			COALESCE(bid_stats.cnt, 0)::bigint AS bidder_count
		FROM auctions a
		LEFT JOIN LATERAL (
			SELECT COUNT(DISTINCT b.bidder_user_id)::bigint AS cnt
			FROM auction_bids b
			WHERE b.auction_id = a.auction_id
		) bid_stats ON true
		WHERE a.seller_id = ?
		  AND a.status = 'active'
		  AND a.end_at > NOW()
		ORDER BY a.end_at ASC, a.auction_id ASC
		LIMIT ? OFFSET ?
	`, sellerID, limit, offset).Scan(ctx, &rows)
	return rows, err
}

func (r auctionRepo) ListSellerReviewsReceived(ctx context.Context, sellerID string, limit int) ([]SellerReviewReceivedRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	sellerID = strings.TrimSpace(sellerID)
	if sellerID == "" {
		return nil, nil
	}
	var rows []SellerReviewReceivedRow
	err := r.bun.NewRaw(`
		SELECT r.auction_id, COALESCE(a.title, '') AS auction_title,
			r.rating::float8 AS rating, r.created_at
		FROM auction_seller_reviews r
		LEFT JOIN auctions a ON a.auction_id = r.auction_id
		WHERE r.seller_id = ?
		ORDER BY r.created_at DESC
		LIMIT ?
	`, sellerID, limit).Scan(ctx, &rows)
	return rows, err
}
