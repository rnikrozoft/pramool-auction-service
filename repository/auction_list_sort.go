package repository

import "strings"

func sqlOrderDir(order string) string {
	if strings.EqualFold(strings.TrimSpace(order), "asc") {
		return "ASC"
	}
	return "DESC"
}

func activeBidSortSQL(sort, order, userID string) string {
	dir := sqlOrderDir(order)
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "start":
		return "u.start_price " + dir + ", u.auction_id DESC"
	case "step":
		return "u.bid_step " + dir + ", u.auction_id DESC"
	case "my_bid":
		return "u.held_amount " + dir + ", u.auction_id DESC"
	case "price":
		return "u.current_bid " + dir + ", u.auction_id DESC"
	case "status":
		uid := strings.ReplaceAll(strings.TrimSpace(userID), "'", "")
		return `(CASE
			WHEN u.can_confirm_received THEN 0
			WHEN u.end_at > NOW() AND u.leading_user_id = '` + uid + `' THEN 1
			WHEN u.end_at > NOW() THEN 2
			WHEN u.leading_user_id = '` + uid + `' THEN 3
			ELSE 4
		END) ` + dir + ", u.end_at DESC, u.auction_id DESC"
	case "end":
		if dir == "ASC" {
			return `(u.end_at > NOW()) DESC,
				(CASE WHEN u.end_at > NOW() THEN u.end_at END) DESC NULLS LAST,
				u.end_at ASC NULLS LAST,
				u.auction_id DESC`
		}
		return `(u.end_at > NOW()) DESC,
			(CASE WHEN u.end_at > NOW() THEN u.end_at END) ASC NULLS LAST,
			u.end_at DESC NULLS LAST,
			u.auction_id DESC`
	default:
		if dir == "ASC" {
			return "u.created_at ASC, u.auction_id ASC"
		}
		return "u.created_at DESC, u.auction_id DESC"
	}
}

func sellerAuctionSortSQL(sort, order string) string {
	dir := sqlOrderDir(order)
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "start":
		return "start_price " + dir + ", created_at DESC, auction_id DESC"
	case "step":
		return "bid_step " + dir + ", created_at DESC, auction_id DESC"
	case "bidders":
		return "COALESCE(bid_stats.cnt, 0) " + dir + ", created_at DESC, auction_id DESC"
	case "status":
		return `(CASE WHEN status = 'active' AND end_at > NOW() THEN 0 ELSE 1 END) ` + dir + ", end_at DESC, auction_id DESC"
	case "price":
		return "current_bid " + dir + ", created_at DESC, auction_id DESC"
	case "end":
		if dir == "ASC" {
			return `(CASE WHEN status = 'active' AND end_at > NOW() THEN 0 ELSE 1 END) ASC,
				(CASE WHEN status = 'active' AND end_at > NOW() THEN end_at END) DESC NULLS LAST,
				end_at ASC NULLS LAST,
				auction_id DESC`
		}
		return `(CASE WHEN status = 'active' AND end_at > NOW() THEN 0 ELSE 1 END) ASC,
			(CASE WHEN status = 'active' AND end_at > NOW() THEN end_at END) ASC NULLS LAST,
			end_at DESC NULLS LAST,
			auction_id DESC`
	default:
		if dir == "ASC" {
			return "created_at ASC, auction_id ASC"
		}
		return "created_at DESC, auction_id DESC"
	}
}
