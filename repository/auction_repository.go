package repository

import (
	"context"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

type AuctionRepository interface {
	BeginTx(ctx context.Context) (bun.Tx, error)

	GetAuctionByID(ctx context.Context, auctionID string) (*entity.Auction, error)
	ListAuctionImages(ctx context.Context, auctionID string) ([]entity.AuctionImage, error)
	FindBidderBySubject(ctx context.Context, subject string) (*entity.Bidder, error)

	LockAuctionForSettlement(ctx context.Context, tx bun.Tx, auctionID string) (AuctionSettlementLock, error)
	SelectWinningBidHold(ctx context.Context, tx bun.Tx, auctionID string) (userID string, heldAmount int64, err error)
	CloseAuctionNoWinner(ctx context.Context, tx bun.Tx, auctionID string) error
	SelectLosingBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) ([]LosingBidHold, error)
	InsertSellerEarningSettled(ctx context.Context, tx bun.Tx, sellerID, auctionID, winnerUserID string, amount int64) error
	InsertBidRefundTransaction(ctx context.Context, tx bun.Tx, userID, auctionID string, amount, balanceBefore, balanceAfter int64) error
	ReleaseNonWinningBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	SettleWinningBidHold(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	CloseAuctionWithWinner(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error

	SelectBidHoldForUpdate(ctx context.Context, tx bun.Tx, auctionID, bidderID string) (oldHeldAmount int64, err error)
	LockUserCredit(ctx context.Context, tx bun.Tx, userID string) (credit int64, err error)
	SetUserCredit(ctx context.Context, tx bun.Tx, userID string, credit int64) error
	UpdateAuctionOnBid(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) (*entity.Auction, error)
	InsertAuctionBidRecord(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error
	UpsertAuctionBidHold(ctx context.Context, tx bun.Tx, auctionID, bidderID string, heldAmount int64) error
	InsertBidHoldAdjustmentTransaction(ctx context.Context, tx bun.Tx, bidderID, auctionID string, delta, balanceBefore, balanceAfter int64) error
}

type auctionRepo struct {
	bun *bun.DB
}

func NewAuctionRepository(b *bun.DB) AuctionRepository {
	return auctionRepo{bun: b}
}

func (r auctionRepo) BeginTx(ctx context.Context) (bun.Tx, error) {
	return r.bun.BeginTx(ctx, nil)
}

func (r auctionRepo) GetAuctionByID(ctx context.Context, auctionID string) (*entity.Auction, error) {
	item := new(entity.Auction)
	query := `
	SELECT auction_id, seller_id, title, category, item_condition AS condition, description,
		start_price, bid_step, current_bid, total_bids, status, end_at, cover_image_url,
		created_at, updated_at, COALESCE(winner_id, '') AS winner_id
	FROM auctions
	WHERE auction_id = ?
	LIMIT 1
	`
	err := r.bun.NewRaw(query, auctionID).Scan(ctx, item)
	return item, err
}

func (r auctionRepo) ListAuctionImages(ctx context.Context, auctionID string) ([]entity.AuctionImage, error) {
	items := make([]entity.AuctionImage, 0)
	query := `
	SELECT id, auction_id, image_url, sort_order, created_at
	FROM auction_images
	WHERE auction_id = ?
	ORDER BY sort_order ASC, id ASC
	`
	err := r.bun.NewRaw(query, auctionID).Scan(ctx, &items)
	return items, err
}
