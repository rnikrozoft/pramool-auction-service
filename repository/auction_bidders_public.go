package repository

import (
	"context"
	"strings"
	"time"
)

// AuctionBidderPublicRow is latest bid per bidder with name fields from users.
type AuctionBidderPublicRow struct {
	BidderUserID string    `bun:"bidder_user_id"`
	BidAmount    int64     `bun:"bid_amount"`
	PlacedAt     time.Time `bun:"placed_at"`
	FirstName    string    `bun:"first_name"`
	LastName     string    `bun:"last_name"`
}

func (r auctionRepo) ListAuctionBiddersPublic(ctx context.Context, auctionID string, limit int) ([]AuctionBidderPublicRow, error) {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var rows []AuctionBidderPublicRow
	err := r.bun.NewRaw(`
		SELECT bidder_user_id, bid_amount, placed_at, first_name, last_name
		FROM (
			SELECT
				b.bidder_user_id,
				b.bid_amount,
				b.placed_at,
				COALESCE(u.first_name, '') AS first_name,
				COALESCE(u.last_name, '') AS last_name,
				ROW_NUMBER() OVER (
					PARTITION BY b.bidder_user_id
					ORDER BY b.bid_amount DESC, b.placed_at DESC
				) AS rn
			FROM auction_bids b
			LEFT JOIN users u ON u.user_id = b.bidder_user_id
			WHERE b.auction_id = ?
		) ranked
		WHERE rn = 1
		ORDER BY bid_amount DESC, placed_at DESC
		LIMIT ?
	`, auctionID, limit).Scan(ctx, &rows)
	return rows, err
}

func (r auctionRepo) ListAuctionBiddersFromParticipants(ctx context.Context, auctionID string, limit int) ([]AuctionBidderPublicRow, error) {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var rows []AuctionBidderPublicRow
	err := r.bun.NewRaw(`
		SELECT
			p.bidder_user_id,
			p.max_bid_amount AS bid_amount,
			p.last_bid_at AS placed_at,
			COALESCE(u.first_name, '') AS first_name,
			COALESCE(u.last_name, '') AS last_name
		FROM auction_bid_participants p
		LEFT JOIN users u ON u.user_id = p.bidder_user_id
		WHERE p.auction_id = ?
		ORDER BY p.max_bid_amount DESC, p.last_bid_at DESC
		LIMIT ?
	`, auctionID, limit).Scan(ctx, &rows)
	return rows, err
}
