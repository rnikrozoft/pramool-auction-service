package repository

import (
	"context"
	"time"
)

// MyBidHistoryRow is one auction the user has placed at least one bid on.
type MyBidHistoryRow struct {
	AuctionID     string
	Title         string
	Category      string
	CoverImageURL string
	Status        string
	CurrentBid    int64
	EndAt         time.Time
	WinnerID      string
	MyMaxBid      int64
	LastBidAt     time.Time
}

func (r auctionRepo) ListMyBidHistory(ctx context.Context, userID string, limit, offset int) ([]MyBidHistoryRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.bun.QueryContext(ctx, `
		SELECT
			a.auction_id,
			a.title,
			a.category,
			COALESCE(a.cover_image_url, ''),
			a.status,
			a.current_bid,
			a.end_at,
			COALESCE(a.winner_id, ''),
			ub.my_max_bid,
			ub.last_bid_at
		FROM (
			SELECT
				auction_id,
				MAX(bid_amount) AS my_max_bid,
				MAX(placed_at) AS last_bid_at
			FROM auction_bids
			WHERE bidder_user_id = ?
			GROUP BY auction_id
		) ub
		INNER JOIN auctions a ON a.auction_id = ub.auction_id
		WHERE a.seller_id <> ?
		ORDER BY ub.last_bid_at DESC
		LIMIT ? OFFSET ?
	`, userID, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MyBidHistoryRow, 0)
	for rows.Next() {
		var row MyBidHistoryRow
		if err := rows.Scan(
			&row.AuctionID,
			&row.Title,
			&row.Category,
			&row.CoverImageURL,
			&row.Status,
			&row.CurrentBid,
			&row.EndAt,
			&row.WinnerID,
			&row.MyMaxBid,
			&row.LastBidAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
