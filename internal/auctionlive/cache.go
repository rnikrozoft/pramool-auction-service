package auctionlive

import (
	"context"
	"time"
)

// BidderEntry is one bidder's latest bid on an open auction (live cache).
type BidderEntry struct {
	BidderUserID string
	BidAmount    int64
	PlacedAt     time.Time
	FirstName    string
	LastName     string
}

// Cache stores live bidder leaderboard per auction (one entry per user, not per click).
type Cache interface {
	Enabled() bool
	UpsertBidder(ctx context.Context, auctionID string, auctionEndAt time.Time, entry BidderEntry) error
	ListBidders(ctx context.Context, auctionID string, limit int) ([]BidderEntry, error)
	ClearAuction(ctx context.Context, auctionID string) error
}

type noopCache struct{}

func (noopCache) Enabled() bool { return false }

func (noopCache) UpsertBidder(context.Context, string, time.Time, BidderEntry) error {
	return nil
}

func (noopCache) ListBidders(context.Context, string, int) ([]BidderEntry, error) {
	return nil, nil
}

func (noopCache) ClearAuction(context.Context, string) error { return nil }

// Noop returns a disabled cache (Postgres-only live bidders).
func Noop() Cache { return noopCache{} }
