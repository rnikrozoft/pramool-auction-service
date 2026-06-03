package dto

// PublicUserProfileResponse is returned for GET /public/users/:id (no auth).
type PublicUserProfileResponse struct {
	UserID              string                   `json:"user_id"`
	DisplayName         string                   `json:"display_name"`
	MemberSince         string                   `json:"member_since"` // RFC3339
	ReviewAvgRating   float64 `json:"review_avg_rating"` // 0.5–5 from reputation_points; 0 = none
	ReviewCount       int     `json:"review_count"`
	SellerNoShipCount int     `json:"seller_no_ship_count"`
	ActiveAuctions      []AuctionListItem        `json:"active_auctions"`
	ActiveAuctionsTotal int                      `json:"active_auctions_total"`
	Reviews             []PublicSellerReviewItem `json:"reviews"`
}

// PublicSellerReviewItem is one buyer review the seller received.
type PublicSellerReviewItem struct {
	AuctionID    string  `json:"auction_id"`
	AuctionTitle string  `json:"auction_title"`
	Rating       float64 `json:"rating"`
	Comment      string  `json:"comment,omitempty"`
	CreatedAt    string  `json:"created_at"` // RFC3339
}
