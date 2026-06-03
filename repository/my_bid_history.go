package repository

import (
	"context"
	"strings"
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
	Outcome       string
}

// MyBidHistoryTabCounts are badge counts for the buyer bid-history dashboard.
type MyBidHistoryTabCounts struct {
	All    int
	Active int
	Outbid int
	Won    int
	Lost   int
}

func myBidHistoryBaseSQL() string {
	return `
		SELECT
			a.auction_id,
			a.title,
			a.category,
			COALESCE(a.cover_image_url, '') AS cover_image_url,
			a.status,
			a.current_bid,
			a.end_at,
			COALESCE(a.winner_id::text, '') AS winner_id,
			ub.my_max_bid,
			ub.last_bid_at,
			CASE
				WHEN a.status = 'active' AND a.end_at > NOW() AND ub.my_max_bid < a.current_bid THEN 'outbid'
				WHEN a.status = 'active' AND a.end_at > NOW() THEN 'active'
				WHEN COALESCE(a.winner_id::text, '') = ? THEN 'won'
				ELSE 'lost'
			END AS outcome
		FROM (
			SELECT
				auction_id,
				max_bid_amount AS my_max_bid,
				last_bid_at
			FROM auction_bid_participants
			WHERE bidder_user_id = ?
		) ub
		INNER JOIN auctions a ON a.auction_id = ub.auction_id
		WHERE a.seller_id <> ?
	`
}

func bidHistoryScopeSQL(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "active":
		return " AND u.outcome = 'active' "
	case "outbid":
		return " AND u.outcome = 'outbid' "
	case "won":
		return " AND u.outcome = 'won' "
	case "lost":
		return " AND u.outcome = 'lost' "
	default:
		return ""
	}
}

func bidHistorySearchSQL(q string) (clause string, args []interface{}) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	pattern := "%" + q + "%"
	return ` AND (u.title ILIKE ? OR u.auction_id ILIKE ?)`, []interface{}{pattern, pattern}
}

func bidHistorySortSQL(sort, order string) string {
	dir := sqlOrderDir(order)
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "my_bid":
		return "u.my_max_bid " + dir + ", u.auction_id DESC"
	case "price":
		return "u.current_bid " + dir + ", u.auction_id DESC"
	case "status":
		return `(CASE u.outcome
			WHEN 'active' THEN 0
			WHEN 'outbid' THEN 1
			WHEN 'won' THEN 2
			ELSE 3
		END) ` + dir + ", u.last_bid_at DESC, u.auction_id DESC"
	case "end":
		return "u.end_at " + dir + ", u.auction_id DESC"
	default:
		return "u.last_bid_at " + dir + ", u.auction_id DESC"
	}
}

func myBidHistoryBaseArgs(userID string) []interface{} {
	return []interface{}{userID, userID, userID}
}

func (r auctionRepo) ListMyBidHistory(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) ([]MyBidHistoryRow, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	scopeClause := bidHistoryScopeSQL(scope)
	searchClause, searchArgs := bidHistorySearchSQL(q)
	orderBy := bidHistorySortSQL(sort, order)
	query := `
		SELECT
			u.auction_id,
			u.title,
			u.category,
			u.cover_image_url,
			u.status,
			u.current_bid,
			u.end_at,
			u.winner_id,
			u.my_max_bid,
			u.last_bid_at,
			u.outcome
		FROM (` + myBidHistoryBaseSQL() + `) AS u
		WHERE 1=1` + scopeClause + searchClause + `
		ORDER BY ` + orderBy + `
		LIMIT ? OFFSET ?
	`
	args := myBidHistoryBaseArgs(userID)
	args = append(args, searchArgs...)
	args = append(args, limit, offset)

	rows, err := r.bun.QueryContext(ctx, query, args...)
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
			&row.Outcome,
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

func (r auctionRepo) CountMyBidHistoryScoped(ctx context.Context, userID, scope, q string) (int, error) {
	scopeClause := bidHistoryScopeSQL(scope)
	searchClause, searchArgs := bidHistorySearchSQL(q)
	query := `
		SELECT COUNT(*)::int
		FROM (` + myBidHistoryBaseSQL() + `) AS u
		WHERE 1=1` + scopeClause + searchClause
	args := myBidHistoryBaseArgs(userID)
	args = append(args, searchArgs...)
	var n int
	err := r.bun.NewRaw(query, args...).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) CountMyBidHistoryTabs(ctx context.Context, userID, q string) (MyBidHistoryTabCounts, error) {
	searchClause, searchArgs := bidHistorySearchSQL(q)
	query := `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE u.outcome = 'active')::int,
			COUNT(*) FILTER (WHERE u.outcome = 'outbid')::int,
			COUNT(*) FILTER (WHERE u.outcome = 'won')::int,
			COUNT(*) FILTER (WHERE u.outcome = 'lost')::int
		FROM (` + myBidHistoryBaseSQL() + `) AS u
		WHERE 1=1` + searchClause
	args := myBidHistoryBaseArgs(userID)
	args = append(args, searchArgs...)
	var counts MyBidHistoryTabCounts
	err := r.bun.NewRaw(query, args...).Scan(ctx, &counts.All, &counts.Active, &counts.Outbid, &counts.Won, &counts.Lost)
	return counts, err
}
