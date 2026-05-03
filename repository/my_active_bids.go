package repository

import (
	"context"
	"time"
)

// MyActiveBidRow is one auction the user still has an active bid hold on.
type MyActiveBidRow struct {
	AuctionID     string
	Title         string
	Category      string
	CoverImageURL string
	CurrentBid    int64
	BidStep       int64
	EndAt         time.Time
	MyHeldAmount  int64
	LeadingUserID string
}

func (r auctionRepo) ListMyActiveBids(ctx context.Context, userID string) ([]MyActiveBidRow, error) {
	rows, err := r.bun.QueryContext(ctx, `
		SELECT
			a.auction_id,
			a.title,
			a.category,
			COALESCE(a.cover_image_url, ''),
			a.current_bid,
			a.bid_step,
			a.end_at,
			h.held_amount,
			COALESCE((
				SELECT h2.user_id
				FROM auction_bid_holds h2
				WHERE h2.auction_id = a.auction_id AND h2.hold_status = 'held'
				ORDER BY h2.held_amount DESC, h2.created_at ASC, h2.user_id ASC
				LIMIT 1
			), '')
		FROM auction_bid_holds h
		INNER JOIN auctions a ON a.auction_id = h.auction_id
		WHERE h.user_id = ?
		  AND h.hold_status = 'held'
		  AND a.status = 'active'
		  AND a.end_at > NOW()
		  AND a.seller_id <> ?
		ORDER BY a.end_at ASC
	`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MyActiveBidRow, 0)
	for rows.Next() {
		var row MyActiveBidRow
		if err := rows.Scan(
			&row.AuctionID,
			&row.Title,
			&row.Category,
			&row.CoverImageURL,
			&row.CurrentBid,
			&row.BidStep,
			&row.EndAt,
			&row.MyHeldAmount,
			&row.LeadingUserID,
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
