package dto

type AuctionDetailResponse struct {
	AuctionID     string   `json:"auction_id"`
	SellerID      string   `json:"seller_id"`
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Condition     string   `json:"condition"`
	Description   string   `json:"description"`
	StartPrice    int64    `json:"start_price"`
	CurrentBid    int64    `json:"current_bid"`
	BidStep       int64    `json:"bid_step"`
	TotalBids     int64    `json:"total_bids"`
	Status        string   `json:"status"`
	EndAt         string   `json:"end_at"`
	CoverImageURL string   `json:"cover_image_url"`
	Images        []string `json:"images"`
}

type PlaceBidResult struct {
	AuctionID       string `json:"auction_id"`
	BidderID        string `json:"bidder_id,omitempty"`
	CurrentBid      int64  `json:"current_bid"`
	TotalBids       int64  `json:"total_bids"`
	RemainingCredit int64  `json:"remaining_credit"`
}
