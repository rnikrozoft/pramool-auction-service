package repository

import (
	"context"
	"time"
)

// MyActiveBidRow is an active bid hold row and/or a closed auction where the buyer may confirm receipt.
type MyActiveBidRow struct {
	AuctionID                 string
	Title                     string
	Category                  string
	CoverImageURL             string
	StartPrice                int64
	CurrentBid                int64
	BidStep                   int64
	EndAt                     time.Time
	MyHeldAmount              int64
	LeadingUserID             string
	AllowEarlyClose           bool
	CanConfirmReceived        bool
	SellerClosePauseBidsUntil *time.Time
}

func (r auctionRepo) ListMyActiveBids(ctx context.Context, userID string) ([]MyActiveBidRow, error) {
	rows, err := r.bun.QueryContext(ctx, `
		SELECT * FROM (
			SELECT
				a.auction_id,
				a.title,
				a.category,
				COALESCE(a.cover_image_url, ''),
				a.start_price,
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
				), '') AS leading_user_id,
				COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
				FALSE AS can_confirm_received,
				a.seller_close_pause_bids_until AS seller_close_pause_bids_until
			FROM auction_bid_holds h
			INNER JOIN auctions a ON a.auction_id = h.auction_id
			WHERE h.user_id = ?
			  AND h.hold_status = 'held'
			  AND a.status = 'active'
			  AND a.end_at > NOW()
			  AND a.seller_id <> ?

			UNION ALL

			SELECT
				a.auction_id,
				a.title,
				a.category,
				COALESCE(a.cover_image_url, ''),
				a.start_price,
				a.current_bid,
				a.bid_step,
				a.end_at,
				a.current_bid AS held_amount,
				COALESCE(NULLIF(TRIM(BOTH FROM COALESCE(a.winner_id::text, '')), ''), '') AS leading_user_id,
				COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
				TRUE AS can_confirm_received,
				NULL::timestamptz AS seller_close_pause_bids_until
			FROM auctions a
			WHERE a.status = 'closed'
			  AND NULLIF(TRIM(BOTH FROM COALESCE(a.winner_id::text, '')), '') = ?
			  AND a.seller_payout_at IS NULL
			  AND a.seller_shipped_at IS NOT NULL
			  AND a.buyer_received_at IS NULL
		) AS u
		ORDER BY (u.end_at > NOW()) DESC, u.end_at ASC
	`, userID, userID, userID)
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
			&row.StartPrice,
			&row.CurrentBid,
			&row.BidStep,
			&row.EndAt,
			&row.MyHeldAmount,
			&row.LeadingUserID,
			&row.AllowEarlyClose,
			&row.CanConfirmReceived,
			&row.SellerClosePauseBidsUntil,
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
