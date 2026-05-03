package dto

type CreateAuctionRequest struct {
	Title           string `json:"title"`
	Category        string `json:"category"`
	Condition       string `json:"condition"`
	Description     string `json:"description"`
	StartPrice      int64  `json:"start_price"`
	BidStep         int64  `json:"bid_step"`
	EndAt           string `json:"end_at"`
	AllowEarlyClose bool   `json:"allow_early_close"`
	/** 0 = off; when highest bid reaches this amount, auction closes immediately. */
	BuyNowPrice int64 `json:"buy_now_price"`
}

type CreateAuctionResponse struct {
	AuctionID string `json:"auction_id"`
}

type SellerAuctionItem struct {
	AuctionID     string `json:"auction_id"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	Status        string `json:"status"`
	StartPrice    int64  `json:"start_price"`
	CurrentBid    int64  `json:"current_bid"`
	TotalBids     int64  `json:"total_bids"`
	EndAt         string `json:"end_at"`
	CoverImageURL string `json:"cover_image_url"`
	BuyNowPrice   int64  `json:"buy_now_price"`
}
