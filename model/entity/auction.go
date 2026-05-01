package entity

import "time"

type Auction struct {
	AuctionID     string    `db:"auction_id"`
	SellerID      string    `db:"seller_id"`
	Title         string    `db:"title"`
	Category      string    `db:"category"`
	Condition     string    `db:"item_condition"`
	Description   string    `db:"description"`
	StartPrice    int64     `db:"start_price"`
	BidStep       int64     `db:"bid_step"`
	CurrentBid    int64     `db:"current_bid"`
	TotalBids     int64     `db:"total_bids"`
	Status        string    `db:"status"`
	EndAt         time.Time `db:"end_at"`
	CoverImageURL string    `db:"cover_image_url"`
	WinnerID      string    `db:"winner_id"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type AuctionImage struct {
	ID        int64     `db:"id"`
	AuctionID string    `db:"auction_id"`
	ImageURL  string    `db:"image_url"`
	SortOrder int       `db:"sort_order"`
	CreatedAt time.Time `db:"created_at"`
}
