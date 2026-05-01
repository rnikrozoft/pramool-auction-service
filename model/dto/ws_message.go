package dto

// AuctionWSMessage is pushed over GET /ws/auctions/:id.
type AuctionWSMessage struct {
	Type              string `json:"type"`
	AuctionID         string `json:"auction_id,omitempty"`
	BidderID          string `json:"bidder_id,omitempty"`
	Amount            int64  `json:"amount,omitempty"`
	CurrentBid        int64  `json:"current_bid,omitempty"`
	TotalBids         int64  `json:"total_bids,omitempty"`
	RemainingCredit   int64  `json:"remaining_credit,omitempty"`
	Message           string `json:"message,omitempty"`
}
