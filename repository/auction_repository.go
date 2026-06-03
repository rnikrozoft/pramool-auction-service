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
	GetUserDisplayName(ctx context.Context, userID string) (UserDisplayName, error)
	GetUserDisplayNamesByIDs(ctx context.Context, userIDs []string) (map[string]UserDisplayName, error)

	LockAuctionForSettlement(ctx context.Context, tx bun.Tx, auctionID string) (AuctionSettlementLock, error)
	// LockAuctionRowForUpdate locks the auction row for bidding (must be called before LockUserCredit in PlaceBid).
	LockAuctionRowForUpdate(ctx context.Context, tx bun.Tx, auctionID string) (*entity.Auction, error)
	// SealAuctionBiddingEndNow sets end_at to now while status stays active — used when seller starts early close so no new bids match end_at > NOW().
	SealAuctionBiddingEndNow(ctx context.Context, tx bun.Tx, auctionID string) error
	SetSellerClosePauseBidsUntil(ctx context.Context, tx bun.Tx, auctionID string, until time.Time) error
	ClearSellerClosePauseBidsUntil(ctx context.Context, tx bun.Tx, auctionID string) error
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
	MarkSellerPayoutOnly(ctx context.Context, tx bun.Tx, auctionID string) error
	MarkBuyerReceivedOnly(ctx context.Context, tx bun.Tx, auctionID string) error
	MarkBuyerEscrowRefunded(ctx context.Context, tx bun.Tx, auctionID string) error
	LockAuctionForNoShipRefund(ctx context.Context, tx bun.Tx, auctionID string, afterSettledDays int) (NoShipRefundLock, error)
	ReleaseEscrowHoldAsRefunded(ctx context.Context, tx bun.Tx, auctionID, winnerUserID string) error
	AdjustReputationPoints(ctx context.Context, tx bun.Tx, userID string, delta int) error
	IncrementSellerNoShipCount(ctx context.Context, tx bun.Tx, sellerID string) error
	ListFulfillmentSweepAuctionIDs(ctx context.Context, limit int) ([]string, error)
	ListUserFulfillmentAuctionIDs(ctx context.Context, userID string, limit int) ([]string, error)
	InsertAuctionSellerReview(ctx context.Context, tx bun.Tx, auctionID, buyerUserID, sellerID string, rating float64, sellerPoints int, comment string) error
	AddSellerReviewAggregate(ctx context.Context, tx bun.Tx, sellerID string, sellerPoints int) error
	HasAuctionSellerReview(ctx context.Context, tx bun.Tx, auctionID string) (bool, error)
	GetSellerPublicProfile(ctx context.Context, sellerID string) (SellerPublicProfile, error)
	GetPublicUserProfileRow(ctx context.Context, userID string) (SellerPublicProfile, time.Time, error)
	ListSellerActiveAuctionsPublic(ctx context.Context, sellerID string, limit, offset int) ([]PublicAuctionRow, error)
	ListSellerClosedAuctionsPublic(ctx context.Context, sellerID string, limit, offset int) ([]PublicAuctionRow, error)
	CountSellerClosedAuctionsPublic(ctx context.Context, sellerID string) (int, error)
	ListSellerReviewsReceived(ctx context.Context, sellerID string, limit int) ([]SellerReviewReceivedRow, error)
	ListAuctionBiddersPublic(ctx context.Context, auctionID string, limit int) ([]AuctionBidderPublicRow, error)
	// ListAuctionBiddersFromParticipants reads persisted bid history after live auction_bids rows are cleared on settle.
	ListAuctionBiddersFromParticipants(ctx context.Context, auctionID string, limit int) ([]AuctionBidderPublicRow, error)
	ZeroEarlyCloseHold(ctx context.Context, tx bun.Tx, auctionID string) error
	RefundEarlyCloseHold(ctx context.Context, tx bun.Tx, sellerID, auctionID string, amount int64) error
	InsertPlatformSaleFee(ctx context.Context, tx bun.Tx, auctionID, sellerID, winnerUserID string, winnerEscrow, sellerShare, platformFee int64, payoutEarlyClose bool, sellerKeepPct int64) error

	SelectBidHoldForUpdate(ctx context.Context, tx bun.Tx, auctionID, bidderID string) (oldHeldAmount int64, err error)
	LockUserCredit(ctx context.Context, tx bun.Tx, userID string) (credit int64, err error)
	SetUserCredit(ctx context.Context, tx bun.Tx, userID string, credit int64) error
	UpdateAuctionOnBid(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64, newEndAt time.Time) (*entity.Auction, error)
	UpdateAuctionEndAtTx(ctx context.Context, tx bun.Tx, auctionID string, newEndAt time.Time) error
	UpsertAuctionBidLive(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error
	UpsertAuctionBidParticipant(ctx context.Context, tx bun.Tx, auctionID, bidderID string, amount int64) error
	ClearAuctionBidsLive(ctx context.Context, tx bun.Tx, auctionID string) error
	UpsertAuctionBidHold(ctx context.Context, tx bun.Tx, auctionID, bidderID string, heldAmount int64) error
	InsertBidHoldAdjustmentTransaction(ctx context.Context, tx bun.Tx, bidderID, auctionID string, delta, balanceBefore, balanceAfter, bidAmount int64) error

	DeleteAuctionBidLiveTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error
	DeleteAuctionBidParticipantTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error
	DeleteAuctionBidHoldTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error
	RecalculateAuctionCurrentBidTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	InsertBidCancelRefundTx(ctx context.Context, tx bun.Tx, userID, auctionID string, refund, forfeit, balanceBefore, balanceAfter, held int64) error
	InsertBidCancelOptionFeeTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, balanceBefore, balanceAfter int64) error
	InsertBidCancelOptionHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64) error
	GetBidCancelOptionHoldOutstandingTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	InsertBidCancelOptionSettlementTx(ctx context.Context, tx bun.Tx, sellerID, auctionID, txType string, amount int64, note string) error
	InsertAutoRenewOptionHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64) error
	GetAutoRenewOptionHoldOutstandingTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	InsertAutoRenewOptionSettlementTx(ctx context.Context, tx bun.Tx, sellerID, auctionID, txType string, amount int64, note string) error

	// ListMyActiveBids returns paginated active bid rows for the buyer dashboard.
	ListMyActiveBids(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) ([]MyActiveBidRow, error)
	CountMyActiveBidsScoped(ctx context.Context, userID, scope, q string) (int, error)
	CountMyActiveBidTabs(ctx context.Context, userID, q string) (MyActiveBidTabCounts, error)

	// ListMyBidHistory returns paginated bid-history rows for the buyer dashboard.
	ListMyBidHistory(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) ([]MyBidHistoryRow, error)
	CountMyBidHistoryScoped(ctx context.Context, userID, scope, q string) (int, error)
	CountMyBidHistoryTabs(ctx context.Context, userID, q string) (MyBidHistoryTabCounts, error)

	// Seller listing (migrated from pramool-core).
	CreateAuctionWithTx(ctx context.Context, tx bun.Tx, auction entity.Auction) error
	CreateAuctionImagesWithTx(ctx context.Context, tx bun.Tx, images []entity.AuctionImage) error
	ListAuctionsBySellerID(ctx context.Context, sellerID, scope, q, sort, order string, limit, offset int) ([]entity.Auction, error)
	CountAuctionsBySellerID(ctx context.Context, sellerID string) (int, error)
	CountSellerAuctionsDisplayActive(ctx context.Context, sellerID string) (int, error)
	CountAuctionsBySellerIDScoped(ctx context.Context, sellerID, scope, q string) (int, error)
	InsertListingDepositHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64, note string) error
	GetListingDepositHoldAmountTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	InsertListingDepositForfeitTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, forfeitAmount int64, note string) error
	LockAuctionBySellerForUpdate(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (*entity.Auction, error)
	CountAuctionBidsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	CountHeldBidHoldsTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error)
	ApplyAuctionReopenTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string, endAt time.Time) (int64, error)
	ApplyAuctionAutoRenewTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string, endAt time.Time) (int64, error)
	// DeleteClosedAuctionNoBidsTx removes a settled closed listing with no bids and no winner (DB cascades children).
	DeleteClosedAuctionNoBidsTx(ctx context.Context, tx bun.Tx, auctionID, sellerID string) (int64, error)

	ListActiveProductCategoryNames(ctx context.Context) ([]string, error)

	GetAuctionSellerForReport(ctx context.Context, auctionID string) (*AuctionReportSeller, error)
	HasPendingAuctionReport(ctx context.Context, auctionID, reporterUserID string) (bool, error)
	InsertAuctionReport(ctx context.Context, auctionID, sellerID, reporterUserID, reason string) (int64, error)
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
	SELECT auction_id, seller_id, title, category, description,
		start_price, bid_step, current_bid, total_bids, status, end_at,
		COALESCE(allow_early_close, FALSE) AS allow_early_close,
		COALESCE(allow_bid_cancel, FALSE) AS allow_bid_cancel,
		COALESCE(auto_renew, FALSE) AS auto_renew,
		COALESCE(auto_renew_count, 0) AS auto_renew_count,
		COALESCE(early_close_hold_amount, 0) AS early_close_hold_amount,
		COALESCE(buy_now_price, 0) AS buy_now_price,
		cover_image_url,
		COALESCE(winner_id::text, '') AS winner_id,
		seller_shipped_at, buyer_received_at, seller_payout_at,
		COALESCE(carrier_code, '') AS carrier_code,
		COALESCE(carrier_name, '') AS carrier_name,
		COALESCE(tracking_number, '') AS tracking_number,
		COALESCE(shipment_status, 'pending') AS shipment_status,
		COALESCE(payout_early_close, FALSE) AS payout_early_close,
		seller_close_pause_bids_until,
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
