package repository

import "time"

// AuctionSettlementLock is a row from auctions FOR UPDATE during settlement.
type AuctionSettlementLock struct {
	SellerID                   string
	Status                     string
	EndAt                      time.Time
	CurrentBid                 int64
	StartPrice                 int64
	AllowEarlyClose            bool
	AutoRenew                  bool
	TotalBids                  int64
	CreatedAt                  time.Time
	EarlyCloseHoldAmount      int64
	SellerClosePauseBidsUntil *time.Time
}

// LosingBidHold is a non-winning held bid row during settlement.
type LosingBidHold struct {
	UserID     string
	HeldAmount int64
}

// EscrowReleaseLock is a row from auctions FOR UPDATE when the buyer confirms receipt.
type EscrowReleaseLock struct {
	SellerID         string
	WinnerID         string
	StartPrice       int64
	PayoutEarlyClose bool
	SellerShipped     bool
	PayoutDone        bool
	BuyerReceivedDone bool
	SellerShippedAt         *time.Time // nil if not yet shipped
	AdminConfirmDeadlineAt  *time.Time
	ShipmentStatus          string
	CreatedAt         time.Time
	EndAt             time.Time
	AutoRenew         bool
}
