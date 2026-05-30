package dto

// PublicAuctionBidderItem is one bidder's latest bid on an auction (GET /auctions/:id/bidders).
type PublicAuctionBidderItem struct {
	BidderUserID string `json:"bidder_user_id"`
	DisplayName  string `json:"display_name"`
	Initials     string `json:"initials"`
	BidAmount    int64  `json:"bid_amount"`
	PlacedAt     string `json:"placed_at"` // RFC3339
}

// PublicAuctionBiddersResponse is returned for GET /auctions/:id/bidders.
type PublicAuctionBiddersResponse struct {
	Items []PublicAuctionBidderItem `json:"items"`
}
