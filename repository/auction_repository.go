package repository

import (
	"context"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/uptrace/bun"
)

type AuctionRepository interface {
	BeginTx(ctx context.Context) (bun.Tx, error)

	GetAuctionByID(ctx context.Context, auctionID string) (*entity.Auction, error)
	ListPublicAuctions(ctx context.Context, f PublicAuctionFilter) ([]PublicAuctionRow, error)
	CountPublicAuctions(ctx context.Context, f PublicAuctionFilter) (int, error)
	ListAuctionImages(ctx context.Context, auctionID string) ([]entity.AuctionImage, error)
	FindBidderBySubject(ctx context.Context, subject string) (*entity.Bidder, error)

	LockAuctionForSettlement(ctx context.Context, tx bun.Tx, auctionID string) (AuctionSettlementLock, error)
	// LockAuctionRowForUpdate locks the auction row for bidding (must be called before LockUserCredit in PlaceBid).
	LockAuctionRowForUpdate(ctx context.Context, tx bun.Tx, auctionID string) (*entity.Auction, error)
	// SealAuctionBiddingEndNow sets end_at to now while status stays active — used when seller starts early close so no new bids match end_at > NOW().
	SealAuctionBiddingEndNow(ctx context.Context, tx bun.Tx, auctionID string) error
	SelectWinningBidHold(ctx context.Context, tx bun.Tx, auctionID string) (userID string, heldAmount int64, err error)
	CloseAuctionNoWinner(ctx context.Context, tx bun.Tx, auctionID string) error
	SelectLosingBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) ([]LosingBidHold, error)
	InsertBidRefundTransaction(ctx context.Context, tx bun.Tx, userID, auctionID string, amount, balanceBefore, balanceAfter int64) error
	InsertCreditLedgerTransaction(ctx context.Context, tx bun.Tx, userID, auctionID, txType string, amount, balanceBefore, balanceAfter int64, note string) error
	ReleaseNonWinningBidHolds(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	SettleWinningBidHold(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	CloseAuctionWithWinner(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string, payoutEarlyClose bool) error
	MoveWinningHoldToEscrow(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	LockAuctionForEscrowRelease(ctx context.Context, tx bun.Tx, auctionID string) (EscrowReleaseLock, error)
	GetWinnerEscrowHoldAmount(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) (int64, error)
	MarkAuctionDeliveryCompleted(ctx context.Context, tx bun.Tx, auctionID string) error
	MarkSellerShipped(ctx context.Context, auctionID, sellerID string) (int64, error)
	ZeroEarlyCloseHold(ctx context.Context, tx bun.Tx, auctionID string) error
	RefundEarlyCloseHold(ctx context.Context, tx bun.Tx, sellerID, auctionID string, amount int64) error

	SelectBidHoldForUpdate(ctx context.Context, tx bun.Tx, auctionID, bidderID string) (oldHeldAmount int64, err error)
	LockUserCredit(ctx context.Context, tx bun.Tx, userID string) (credit int64, err error)
	SetUserCredit(ctx context.Context, tx bun.Tx, userID string, credit int64) error
	UpdateAuctionOnBid(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) (*entity.Auction, error)
	InsertAuctionBidRecord(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error
	UpsertAuctionBidHold(ctx context.Context, tx bun.Tx, auctionID, bidderID string, heldAmount int64) error
	InsertBidHoldAdjustmentTransaction(ctx context.Context, tx bun.Tx, bidderID, auctionID string, delta, balanceBefore, balanceAfter, bidAmount int64) error

	// ListMyActiveBids returns auctions where the user still has hold_status='held' and bidding is open.
	ListMyActiveBids(ctx context.Context, userID string) ([]MyActiveBidRow, error)

	// ListMyBidHistory returns auctions the user has placed at least one bid on (excludes own listings).
	ListMyBidHistory(ctx context.Context, userID string, limit, offset int) ([]MyBidHistoryRow, error)

	// Seller listing (migrated from pramool-core).
	CreateAuctionWithTx(ctx context.Context, tx bun.Tx, auction entity.Auction) error
	CreateAuctionImagesWithTx(ctx context.Context, tx bun.Tx, images []entity.AuctionImage) error
	ListAuctionsBySellerID(ctx context.Context, sellerID string) ([]entity.Auction, error)
	InsertListingDepositHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64, note string) error
	LockAuctionBySellerForUpdate(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (*entity.Auction, error)
	CountAuctionBidsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	CountHeldBidHoldsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	ApplyAuctionReopenTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string, endAt time.Time) (int64, error)
	// DeleteClosedAuctionNoBidsTx removes a settled closed listing with no bids and no winner (DB cascades children).
	DeleteClosedAuctionNoBidsTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (int64, error)
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
		start_price, bid_step, current_bid, total_bids, status, end_at,
		COALESCE(allow_early_close, FALSE) AS allow_early_close,
		COALESCE(early_close_hold_amount, 0) AS early_close_hold_amount,
		COALESCE(buy_now_price, 0) AS buy_now_price,
		cover_image_url,
		COALESCE(winner_id, '') AS winner_id,
		seller_shipped_at, buyer_received_at, seller_payout_at,
		COALESCE(payout_early_close, FALSE) AS payout_early_close,
		created_at, updated_at
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
