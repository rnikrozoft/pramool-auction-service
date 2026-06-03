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
	Status        string `json:"status"`
	CoverImageURL   string `json:"cover_image_url"`
	BuyNowPrice           int64   `json:"buy_now_price"`
	AllowEarlyClose       bool    `json:"allow_early_close"`
	AllowBidCancel        bool    `json:"allow_bid_cancel"`
	SellerID              string  `json:"seller_id,omitempty"`
	SellerDisplayName     string  `json:"seller_display_name,omitempty"`
	SellerReviewAvgRating float64 `json:"seller_review_avg_rating,omitempty"`
	SellerReviewCount     int     `json:"seller_review_count,omitempty"`
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
	Description           string   `json:"description"`
	StartPrice            int64    `json:"start_price"`
	CurrentBid            int64    `json:"current_bid"`
	BidStep               int64    `json:"bid_step"`
	TotalBids             int64    `json:"total_bids"`
	Status                string   `json:"status"`
	EndAt                 string   `json:"end_at"`
	AllowEarlyClose       bool     `json:"allow_early_close"`
	AllowBidCancel        bool     `json:"allow_bid_cancel"`
	AutoRenew             bool     `json:"auto_renew"`
	ReopenEligible        bool     `json:"reopen_eligible"`
	CoverImageURL         string   `json:"cover_image_url"`
	Images                []string `json:"images"`
	SellerShippedAt         string `json:"seller_shipped_at,omitempty"`
	CarrierCode             string `json:"carrier_code,omitempty"`
	CarrierName             string `json:"carrier_name,omitempty"`
	TrackingNumber          string `json:"tracking_number,omitempty"`
	ShipmentStatus          string `json:"shipment_status,omitempty"`
	CanConfirmReceived      bool   `json:"can_confirm_received"`
	BuyerReceivedAt         string `json:"buyer_received_at,omitempty"`
	SellerPayoutAt          string `json:"seller_payout_at,omitempty"`
	PendingSellerPayout     bool   `json:"pending_seller_payout"`
	EscrowAutoConfirmDays   int    `json:"escrow_auto_confirm_days,omitempty"`
	EscrowAutoConfirmAt     string `json:"escrow_auto_confirm_at,omitempty"`
	/** 0 = not set; bid >= this closes auction immediately. */
	BuyNowPrice int64 `json:"buy_now_price"`
	/** RFC3339 — ถ้ามีและเวลาปัจจุบันยังไม่ถึง ระบบไม่รับบิด (ผู้ขายกำลังปิดประมูล) */
	BiddingPausedUntil string `json:"bidding_paused_until,omitempty"`
	SellerDisplayName     string  `json:"seller_display_name,omitempty"`
	SellerReviewAvgRating float64 `json:"seller_review_avg_rating,omitempty"` // 0.5–5 stars; 0 = no reviews
	SellerReviewCount     int     `json:"seller_review_count,omitempty"`
}

type PlaceBidResult struct {
	AuctionID       string `json:"auction_id"`
	BidderID        string `json:"bidder_id,omitempty"`
	CurrentBid      int64  `json:"current_bid"`
	TotalBids       int64  `json:"total_bids"`
	RemainingCredit int64  `json:"remaining_credit"`
	AuctionClosed   bool   `json:"auction_closed"`
	/** RFC3339 — เวลาปิดหลังขยายจากการบิด (ไม่ส่งเมื่อปิดด้วย buy-now ทันที) */
	EndAt string `json:"end_at,omitempty"`
}

type CancelBidResult struct {
	AuctionID       string `json:"auction_id"`
	RefundedBaht    int64  `json:"refunded_baht"`
	ForfeitedBaht   int64  `json:"forfeited_baht"`
	RemainingCredit int64  `json:"remaining_credit"`
	CurrentBid      int64  `json:"current_bid"`
	EndAt           string `json:"end_at"`
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
	ShipmentStatus     string `json:"shipment_status,omitempty"`
	BiddingPausedUntil   string `json:"bidding_paused_until,omitempty"`
	CreatedAt            string `json:"created_at"`
}

type MyActiveBidsResponse struct {
	Items           []MyActiveBidItem `json:"items"`
	Total           int               `json:"total"`
	AllCount        int               `json:"all_count"`
	ActiveCount     int               `json:"active_count"`
	EndingSoonCount int               `json:"ending_soon_count"`
	OutbidCount     int               `json:"outbid_count"`
	ClosedCount     int               `json:"closed_count"`
	Limit           int               `json:"limit"`
	Offset          int               `json:"offset"`
	Scope           string            `json:"scope"`
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
	EndAt          string `json:"end_at"`
}

type MyBidHistoryResponse struct {
	Items        []MyBidHistoryItem `json:"items"`
	Total        int                `json:"total"`
	AllCount     int                `json:"all_count"`
	ActiveCount  int                `json:"active_count"`
	OutbidCount  int                `json:"outbid_count"`
	WonCount     int                `json:"won_count"`
	LostCount    int                `json:"lost_count"`
	Limit        int                `json:"limit"`
	Offset       int                `json:"offset"`
	Scope        string             `json:"scope"`
}
