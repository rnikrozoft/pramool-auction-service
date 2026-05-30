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
	SellerClosePauseBidsUntil *time.Time
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
			COALESCE(a.cover_image_url, '') AS cover_image_url,
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

func activeBidSortSQL(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "price":
		return "u.current_bid DESC, u.auction_id DESC"
	case "end":
		return `(u.end_at > NOW()) DESC,
			(CASE WHEN u.end_at > NOW() THEN u.end_at END) ASC NULLS LAST,
			u.end_at DESC NULLS LAST,
			u.auction_id DESC`
	default:
		return "u.auction_id DESC"
	}
}

func (r auctionRepo) ListMyActiveBids(ctx context.Context, userID, scope, q, sort string, limit, offset int) ([]MyActiveBidRow, error) {
	scopeClause := activeBidScopeSQL(scope, userID)
	searchClause, searchArgs := activeBidSearchSQL(q)
	orderBy := activeBidSortSQL(sort)
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
			u.seller_close_pause_bids_until
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
