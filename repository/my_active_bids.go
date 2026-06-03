package repository

import (
	"context"
	"strings"
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
	ShipmentStatus            string
	SellerClosePauseBidsUntil *time.Time
	CreatedAt                 time.Time
}

// MyActiveBidTabCounts are badge counts for the buyer active-bids dashboard.
type MyActiveBidTabCounts struct {
	All        int
	Active     int
	EndingSoon int
	Outbid     int
	Closed     int
}

func myActiveBidsUnionSQL() string {
	return `
		SELECT
			a.auction_id,
			a.title,
			a.category,
			COALESCE(a.cover_image_url, '') AS cover_image_url,
			a.start_price,
			a.current_bid,
			a.bid_step,
			a.end_at,
			h.held_amount,
			COALESCE((
				SELECT h2.user_id::text
				FROM auction_bid_holds h2
				WHERE h2.auction_id = a.auction_id AND h2.hold_status = 'held'
				ORDER BY h2.held_amount DESC, h2.created_at ASC, h2.user_id ASC
				LIMIT 1
			), '') AS leading_user_id,
			COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
			FALSE AS can_confirm_received,
			'pending' AS shipment_status,
			a.seller_close_pause_bids_until AS seller_close_pause_bids_until,
			a.created_at
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
			COALESCE(a.cover_image_url, '') AS cover_image_url,
			a.start_price,
			a.current_bid,
			a.bid_step,
			a.end_at,
			a.current_bid AS held_amount,
			COALESCE(a.winner_id::text, '') AS leading_user_id,
			COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
			(a.shipment_status = 'delivered') AS can_confirm_received,
			COALESCE(a.shipment_status, 'pending') AS shipment_status,
			NULL::timestamptz AS seller_close_pause_bids_until,
			a.created_at
		FROM auctions a
		WHERE a.status = 'closed'
		  AND a.winner_id IS NOT NULL
		  AND a.winner_id = ?
		  AND a.buyer_received_at IS NULL
		  AND a.buyer_escrow_refunded_at IS NULL
	`
}

func activeBidScopeSQL(scope, userID string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "active":
		return " AND u.end_at > NOW() "
	case "ending_soon":
		return " AND u.end_at > NOW() AND u.end_at <= NOW() + INTERVAL '2 hours' "
	case "outbid":
		return " AND u.end_at > NOW() AND u.leading_user_id <> ? "
	case "closed":
		return " AND (u.end_at <= NOW() OR u.can_confirm_received = TRUE) "
	default:
		return ""
	}
}

func activeBidSearchSQL(q string) (clause string, args []interface{}) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	pattern := "%" + q + "%"
	return ` AND (u.title ILIKE ? OR u.auction_id ILIKE ?)`, []interface{}{pattern, pattern}
}

func (r auctionRepo) ListMyActiveBids(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) ([]MyActiveBidRow, error) {
	scopeClause := activeBidScopeSQL(scope, userID)
	searchClause, searchArgs := activeBidSearchSQL(q)
	orderBy := activeBidSortSQL(sort, order, userID)
	query := `
		SELECT
			u.auction_id,
			u.title,
			u.category,
			u.cover_image_url,
			u.start_price,
			u.current_bid,
			u.bid_step,
			u.end_at,
			u.held_amount,
			u.leading_user_id,
			u.allow_early_close,
			u.can_confirm_received,
			u.shipment_status,
			u.seller_close_pause_bids_until,
			u.created_at
		FROM (` + myActiveBidsUnionSQL() + `) AS u
		WHERE 1=1` + scopeClause + searchClause + `
		ORDER BY ` + orderBy + `
		LIMIT ? OFFSET ?
	`
	args := []interface{}{userID, userID, userID}
	if strings.EqualFold(strings.TrimSpace(scope), "outbid") {
		args = append(args, userID)
	}
	args = append(args, searchArgs...)
	args = append(args, limit, offset)

	rows, err := r.bun.QueryContext(ctx, query, args...)
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
			&row.ShipmentStatus,
			&row.SellerClosePauseBidsUntil,
			&row.CreatedAt,
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

func (r auctionRepo) CountMyActiveBidsScoped(ctx context.Context, userID, scope, q string) (int, error) {
	scopeClause := activeBidScopeSQL(scope, userID)
	searchClause, searchArgs := activeBidSearchSQL(q)
	query := `
		SELECT COUNT(*)::int
		FROM (` + myActiveBidsUnionSQL() + `) AS u
		WHERE 1=1` + scopeClause + searchClause
	args := []interface{}{userID, userID, userID}
	if strings.EqualFold(strings.TrimSpace(scope), "outbid") {
		args = append(args, userID)
	}
	args = append(args, searchArgs...)
	var n int
	err := r.bun.NewRaw(query, args...).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) CountMyActiveBidTabs(ctx context.Context, userID, q string) (MyActiveBidTabCounts, error) {
	searchClause, searchArgs := activeBidSearchSQL(q)
	query := `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE u.end_at > NOW())::int,
			COUNT(*) FILTER (WHERE u.end_at > NOW() AND u.end_at <= NOW() + INTERVAL '2 hours')::int,
			COUNT(*) FILTER (WHERE u.end_at > NOW() AND u.leading_user_id <> ?)::int,
			COUNT(*) FILTER (WHERE u.end_at <= NOW() OR u.can_confirm_received = TRUE)::int
		FROM (` + myActiveBidsUnionSQL() + `) AS u
		WHERE 1=1` + searchClause
	args := []interface{}{userID, userID, userID, userID}
	args = append(args, searchArgs...)
	var counts MyActiveBidTabCounts
	err := r.bun.NewRaw(query, args...).Scan(ctx, &counts.All, &counts.Active, &counts.EndingSoon, &counts.Outbid, &counts.Closed)
	return counts, err
}
