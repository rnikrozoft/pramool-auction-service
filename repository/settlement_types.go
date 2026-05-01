package repository

import "time"

// AuctionSettlementLock is a row from auctions FOR UPDATE during settlement.
type AuctionSettlementLock struct {
	SellerID string
	Status   string
	EndAt    time.Time
}

// LosingBidHold is a non-winning held bid row during settlement.
type LosingBidHold struct {
	UserID     string
	HeldAmount int64
}
