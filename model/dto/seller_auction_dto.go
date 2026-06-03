package dto

type CreateAuctionRequest struct {
	Title           string `json:"title"`
	Category        string `json:"category"`
	Description     string `json:"description"`
	StartPrice      int64  `json:"start_price"`
	BidStep         int64  `json:"bid_step"`
	EndAt           string `json:"end_at"`
	AllowEarlyClose bool   `json:"allow_early_close"`
	AllowBidCancel  bool   `json:"allow_bid_cancel"`
	AutoRenew       bool   `json:"auto_renew"`
	/** 0 = off; when highest bid reaches this amount, auction closes immediately. */
	BuyNowPrice int64 `json:"buy_now_price"`
}

type CreateAuctionResponse struct {
	AuctionID string `json:"auction_id"`
}

// SellerAuctionListResponse is the JSON body for GET /seller/auctions (paginated).
type SellerAuctionListResponse struct {
	Items       []SellerAuctionItem `json:"items"`
	Total       int                 `json:"total"`         // rows matching scope (for load more)
	AllCount    int                 `json:"all_count"`     // total listings (tab "ทั้งหมด")
	ActiveCount int                 `json:"active_count"`  // display-active (tab + summary card)
	Limit       int                 `json:"limit"`
	Offset      int                 `json:"offset"`
	Scope       string              `json:"scope"` // all | active | closed
}

type SellerAuctionItem struct {
	AuctionID           string `json:"auction_id"`
	Title               string `json:"title"`
	Category            string `json:"category"`
	Status              string `json:"status"`
	StartPrice          int64  `json:"start_price"`
	BidStep             int64  `json:"bid_step"`
	CurrentBid          int64  `json:"current_bid"`
	TotalBids           int64  `json:"total_bids"`
	BidderCount         int64  `json:"bidder_count"`
	EndAt               string `json:"end_at"`
	CoverImageURL       string `json:"cover_image_url"`
	BuyNowPrice         int64  `json:"buy_now_price"`
	AllowEarlyClose     bool   `json:"allow_early_close"`
	AllowBidCancel      bool   `json:"allow_bid_cancel"`
	AutoRenew           bool   `json:"auto_renew"`
	ReopenEligible      bool   `json:"reopen_eligible"`
	PendingSellerPayout bool   `json:"pending_seller_payout"`
	SellerShippedAt     string `json:"seller_shipped_at,omitempty"`
	/** RFC3339 — ผู้ขายเริ่มปิดก่อนเวลา ยังไม่รับบิด */
	BiddingPausedUntil string `json:"bidding_paused_until,omitempty"`
	/** ชื่อผู้ชนะ — มีเมื่อปิดประมูลแล้วและมี winner_id */
	WinnerDisplayName string `json:"winner_display_name,omitempty"`
	WinnerID          string `json:"winner_id,omitempty"`
	CreatedAt         string `json:"created_at"`
}
