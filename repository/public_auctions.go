package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PublicAuctionRow is one row for GET /auctions (browse).
type PublicAuctionRow struct {
	AuctionID       string    `bun:"auction_id"`
	Title           string    `bun:"title"`
	Category        string    `bun:"category"`
	StartPrice      int64     `bun:"start_price"`
	CurrentBid      int64     `bun:"current_bid"`
	BidStep         int64     `bun:"bid_step"`
	TotalBids       int64     `bun:"total_bids"`
	EndAt           time.Time `bun:"end_at"`
	Status          string    `bun:"status"`
	CoverImageURL   string    `bun:"cover_image_url"`
	BuyNowPrice     int64     `bun:"buy_now_price"`
	AllowEarlyClose bool      `bun:"allow_early_close"`
	AllowBidCancel  bool      `bun:"allow_bid_cancel"`
	CreatedAt               time.Time `bun:"created_at"`
	BidderCount             int64     `bun:"bidder_count"`
	SellerID                string    `bun:"seller_id"`
	SellerFirstName         string    `bun:"seller_first_name"`
	SellerLastName          string    `bun:"seller_last_name"`
	SellerReputationPoints  int64     `bun:"reputation_points"`
	SellerReviewCount       int       `bun:"seller_review_count"`
}

// PublicAuctionFilter drives ListPublicAuctions / CountPublicAuctions.
// MinPrice/MaxPrice apply to current_bid. MinStartPrice/MaxStartPrice to start_price.
// MinBidStep/MaxBidStep to bid_step. MinSellerRating filters seller avg stars (0.5–5).
// EndFromDate/EndToDate are YYYY-MM-DD in Asia/Bangkok calendar for end_at.
// EndedScope: open = กำลังประมูล/ยังไม่ปิด, closed = ปิดแล้วและไม่มีผู้บิด, any = ทั้งสองแบบ
type PublicAuctionFilter struct {
	Query           string
	Category        string
	EndedScope      string // "", "open", "closed", "any"
	MinPrice      *int64
	MaxPrice      *int64
	MinStartPrice *int64
	MaxStartPrice *int64
	MinBidStep       *int64
	MaxBidStep       *int64
	MinSellerRating  *float64 // 0.5–5 stars; requires seller with reviews
	EndFromDate      string
	EndToDate     string
	Sort          string
	Limit         int
	Offset        int
}

func orderByPublicAuctions(sort string) string {
	switch strings.TrimSpace(strings.ToLower(sort)) {
	case "price_asc":
		return "a.current_bid ASC, a.auction_id ASC"
	case "price_desc":
		return "a.current_bid DESC, a.auction_id DESC"
	case "ending_soon":
		return "a.end_at ASC, a.auction_id ASC"
	case "most_bids":
		return "a.total_bids DESC, a.created_at DESC, a.auction_id DESC"
	case "most_bidders":
		return "COALESCE(bid_stats.cnt, 0) DESC, a.created_at DESC, a.auction_id DESC"
	case "avg_price_asc":
		return "((a.start_price + a.current_bid) / 2) ASC, a.auction_id ASC"
	case "newest":
		fallthrough
	default:
		return "a.created_at DESC, a.auction_id DESC"
	}
}

func publicAuctionFromClause(f PublicAuctionFilter, withBidStats bool) string {
	var b strings.Builder
	b.WriteString("FROM auctions a")
	b.WriteString("\nLEFT JOIN users u ON u.user_id = a.seller_id")
	if withBidStats {
		b.WriteString(`
LEFT JOIN LATERAL (
	SELECT COUNT(DISTINCT b.bidder_user_id)::bigint AS cnt
	FROM auction_bids b
	WHERE b.auction_id = a.auction_id
) bid_stats ON true`)
	}
	return b.String()
}

func (r auctionRepo) CountPublicAuctions(ctx context.Context, f PublicAuctionFilter) (int, error) {
	q, args := buildPublicAuctionWhere(f)
	query := "SELECT COUNT(*)::int " + publicAuctionFromClause(f, false) + " WHERE " + q
	var n int
	err := r.bun.NewRaw(query, args...).Scan(ctx, &n)
	return n, err
}

func (r auctionRepo) ListPublicAuctions(ctx context.Context, f PublicAuctionFilter) ([]PublicAuctionRow, error) {
	q, args := buildPublicAuctionWhere(f)
	order := orderByPublicAuctions(f.Sort)
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	fromClause := publicAuctionFromClause(f, true) + "\nWHERE " + q

	sqlStr := fmt.Sprintf(`
SELECT
	a.auction_id,
	a.title,
	a.category,
	a.start_price,
	a.current_bid,
	a.bid_step,
	a.total_bids,
	a.end_at,
	a.status,
	a.cover_image_url,
	COALESCE(a.buy_now_price, 0)::bigint AS buy_now_price,
	COALESCE(a.allow_early_close, FALSE) AS allow_early_close,
	COALESCE(a.allow_bid_cancel, FALSE) AS allow_bid_cancel,
	a.created_at,
	COALESCE(bid_stats.cnt, 0)::bigint AS bidder_count,
	a.seller_id,
	COALESCE(u.first_name, '') AS seller_first_name,
	COALESCE(u.last_name, '') AS seller_last_name,
	COALESCE(u.reputation_points, 0)::bigint AS reputation_points,
	COALESCE(u.seller_review_count, 0)::int AS seller_review_count
%s
ORDER BY %s
LIMIT ? OFFSET ?
`, fromClause, order)

	args = append(args, limit, offset)

	var rows []PublicAuctionRow
	err := r.bun.NewRaw(sqlStr, args...).Scan(ctx, &rows)
	return rows, err
}

func buildPublicAuctionWhere(f PublicAuctionFilter) (string, []interface{}) {
	var parts []string
	var args []interface{}

	switch strings.ToLower(strings.TrimSpace(f.EndedScope)) {
	case "closed":
		parts = append(parts, "NOT (a.status = 'active' AND a.end_at > NOW())")
		parts = append(parts, "a.total_bids = 0")
	case "any":
		parts = append(parts, "a.status IN ('active', 'closed')")
	default:
		parts = append(parts, "a.status = 'active'")
		parts = append(parts, "a.end_at > NOW()")
	}

	if cat := strings.TrimSpace(f.Category); cat != "" && cat != "ทั้งหมด" {
		// รองรับ multiselect จาก query "cat1,cat2,..." และให้ match อย่างน้อยหนึ่งแท็ก
		cats := strings.Split(cat, ",")
		conds := make([]string, 0, len(cats))
		seen := map[string]bool{}
		for _, raw := range cats {
			c := strings.TrimSpace(raw)
			if c == "" || c == "ทั้งหมด" || seen[c] {
				continue
			}
			seen[c] = true
			conds = append(conds, "? = ANY(string_to_array(a.category, '|'))")
			args = append(args, c)
		}
		if len(conds) > 0 {
			parts = append(parts, "("+strings.Join(conds, " OR ")+")")
		}
	}

	if q := strings.TrimSpace(f.Query); q != "" {
		// Search by title only (partial match). Split words so users can type a subset.
		// Example: "iphone 15" -> title must contain "iphone" AND "15" (in any position).
		tokens := strings.Fields(strings.ToLower(q))
		if len(tokens) == 0 {
			tokens = []string{strings.ToLower(q)}
		}
		for _, tk := range tokens {
			if tk == "" {
				continue
			}
			parts = append(parts, "LOWER(a.title) LIKE ?")
			args = append(args, "%"+tk+"%")
		}
	}

	if f.MinPrice != nil && *f.MinPrice >= 0 {
		parts = append(parts, "a.current_bid >= ?")
		args = append(args, *f.MinPrice)
	}

	if f.MaxPrice != nil && *f.MaxPrice >= 0 {
		parts = append(parts, "a.current_bid <= ?")
		args = append(args, *f.MaxPrice)
	}

	if f.MinStartPrice != nil && *f.MinStartPrice >= 0 {
		parts = append(parts, "a.start_price >= ?")
		args = append(args, *f.MinStartPrice)
	}

	if f.MaxStartPrice != nil && *f.MaxStartPrice >= 0 {
		parts = append(parts, "a.start_price <= ?")
		args = append(args, *f.MaxStartPrice)
	}

	if f.MinBidStep != nil && *f.MinBidStep > 0 {
		parts = append(parts, "a.bid_step >= ?")
		args = append(args, *f.MinBidStep)
	}

	if f.MaxBidStep != nil && *f.MaxBidStep > 0 {
		parts = append(parts, "a.bid_step <= ?")
		args = append(args, *f.MaxBidStep)
	}

	if f.MinSellerRating != nil && *f.MinSellerRating > 0 {
		parts = append(parts, `COALESCE(u.seller_review_count, 0) > 0 AND GREATEST(COALESCE(u.reputation_points, 0)::float8 / u.seller_review_count::float8 / 2.0, 0) >= ?`)
		args = append(args, *f.MinSellerRating)
	}

	if d := strings.TrimSpace(f.EndFromDate); d != "" {
		parts = append(parts, "(a.end_at AT TIME ZONE 'Asia/Bangkok')::date >= ?::date")
		args = append(args, d)
	}

	if d := strings.TrimSpace(f.EndToDate); d != "" {
		parts = append(parts, "(a.end_at AT TIME ZONE 'Asia/Bangkok')::date <= ?::date")
		args = append(args, d)
	}

	return strings.Join(parts, " AND "), args
}
