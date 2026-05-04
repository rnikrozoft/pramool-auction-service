package dto

// AuctionWSMessage is pushed over GET /ws/auctions/:id.
type AuctionWSMessage struct {
	Type            string `json:"type"`
	AuctionID       string `json:"auction_id,omitempty"`
	BidderID        string `json:"bidder_id,omitempty"`
	Amount          int64  `json:"amount,omitempty"`
	CurrentBid      int64  `json:"current_bid,omitempty"`
	TotalBids       int64  `json:"total_bids,omitempty"`
	RemainingCredit int64  `json:"remaining_credit,omitempty"`
	AuctionClosed   bool   `json:"auction_closed,omitempty"`
	Message         string `json:"message,omitempty"`
	// auction_state — listing fields (pointers so we omit on bid_ack / bid_update).
	Status          string `json:"status,omitempty"`
	EndAt           string `json:"end_at,omitempty"`
	ReopenEligible  *bool  `json:"reopen_eligible,omitempty"`
	AllowEarlyClose *bool  `json:"allow_early_close,omitempty"`
	/** RFC3339 — ถ้ามี ฝั่ง client ห้ามส่งบิดจนกว่าจะหมดเวลาหรือได้ auction_state ใหม่ */
	BiddingPausedUntil string `json:"bidding_paused_until,omitempty"`
}
