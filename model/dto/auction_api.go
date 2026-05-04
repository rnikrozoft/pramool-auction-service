package dto

// AuctionListItem is one row in GET /auctions (browse).
type AuctionListItem struct {
	AuctionID     string `json:"auction_id"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	StartPrice    int64  `json:"start_price"`
	CurrentBid    int64  `json:"current_bid"`
	BidStep       int64  `json:"bid_step"`
	TotalBids     int64  `json:"total_bids"`
	BidderCount   int64  `json:"bidder_count"`
	EndAt         string `json:"end_at"`
	CoverImageURL   string `json:"cover_image_url"`
	BuyNowPrice     int64  `json:"buy_now_price"`
	AllowEarlyClose bool   `json:"allow_early_close"`
}

// AuctionListResponse is the JSON body for GET /auctions.
type AuctionListResponse struct {
	Items  []AuctionListItem `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type AuctionDetailResponse struct {
	AuctionID             string   `json:"auction_id"`
	SellerID              string   `json:"seller_id"`
	WinnerID              string   `json:"winner_id"`
	Title                 string   `json:"title"`
	Category              string   `json:"category"`
	Condition             string   `json:"condition"`
	Description           string   `json:"description"`
	StartPrice            int64    `json:"start_price"`
	CurrentBid            int64    `json:"current_bid"`
	BidStep               int64    `json:"bid_step"`
	TotalBids             int64    `json:"total_bids"`
	Status                string   `json:"status"`
	EndAt                 string   `json:"end_at"`
	AllowEarlyClose       bool     `json:"allow_early_close"`
	ReopenEligible        bool     `json:"reopen_eligible"`
	CoverImageURL         string   `json:"cover_image_url"`
	Images                []string `json:"images"`
	SellerShippedAt         string `json:"seller_shipped_at,omitempty"`
	BuyerReceivedAt         string `json:"buyer_received_at,omitempty"`
	SellerPayoutAt          string `json:"seller_payout_at,omitempty"`
	PendingSellerPayout     bool   `json:"pending_seller_payout"`
	EscrowAutoConfirmDays   int    `json:"escrow_auto_confirm_days,omitempty"`
	EscrowAutoConfirmAt     string `json:"escrow_auto_confirm_at,omitempty"`
	/** 0 = not set; bid >= this closes auction immediately. */
	BuyNowPrice int64 `json:"buy_now_price"`
	/** RFC3339 — ถ้ามีและเวลาปัจจุบันยังไม่ถึง ระบบไม่รับบิด (ผู้ขายกำลังปิดประมูล) */
	BiddingPausedUntil string `json:"bidding_paused_until,omitempty"`
}

type PlaceBidResult struct {
	AuctionID       string `json:"auction_id"`
	BidderID        string `json:"bidder_id,omitempty"`
	CurrentBid      int64  `json:"current_bid"`
	TotalBids       int64  `json:"total_bids"`
	RemainingCredit int64  `json:"remaining_credit"`
	AuctionClosed   bool   `json:"auction_closed"`
}

type CloseEarlyRequest struct {
	Reason string `json:"reason"`
}

// MyActiveBidItem is one row for GET /my/active-bids.
type MyActiveBidItem struct {
	AuctionID          string `json:"auction_id"`
	Title              string `json:"title"`
	Category           string `json:"category"`
	CoverImageURL      string `json:"cover_image_url"`
	StartPrice         int64  `json:"start_price"`
	CurrentBid         int64  `json:"current_bid"`
	BidStep            int64  `json:"bid_step"`
	MyHeldAmount       int64  `json:"my_held_amount"`
	NextMinimumBid     int64  `json:"next_minimum_bid"`
	IsLeading          bool   `json:"is_leading"`
	EndAt              string `json:"end_at"`
	AllowEarlyClose    bool   `json:"allow_early_close"`
	CanConfirmReceived bool   `json:"can_confirm_received"`
	BiddingPausedUntil   string `json:"bidding_paused_until,omitempty"`
}

type MyActiveBidsResponse struct {
	Items []MyActiveBidItem `json:"items"`
}

// MyBidHistoryItem is one row for GET /my/bid-history (outcome: active | outbid | won | lost).
type MyBidHistoryItem struct {
	AuctionID      string `json:"auction_id"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	CoverImageURL  string `json:"cover_image_url"`
	Outcome        string `json:"outcome"`
	AuctionStatus  string `json:"auction_status"`
	MyHighestBid   int64  `json:"my_highest_bid"`
	FinalPrice     int64  `json:"final_price"`
	LastBidAt      string `json:"last_bid_at"`
}

type MyBidHistoryResponse struct {
	Items  []MyBidHistoryItem `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}
