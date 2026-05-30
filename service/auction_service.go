package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rnikrozoft/pramool-auction-service/internal/auctionlive"
	"github.com/rnikrozoft/pramool-auction-service/internal/config"
	"github.com/rnikrozoft/pramool-auction-service/internal/money"
	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/model/entity"
	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/uptrace/bun"
)

var (
	ErrBidAmountTooLow       = errors.New("bid amount is too low")
	ErrCannotBidOwn          = errors.New("cannot bid own auction")
	ErrAuctionClosed         = errors.New("auction is closed")
	ErrInsufficientCredit    = errors.New("insufficient credit")
	ErrCannotCloseEarly      = errors.New("cannot close auction early")
	ErrNotAuctionSeller      = errors.New("not auction seller")
	ErrNotAuctionWinner      = errors.New("not auction winner")
	ErrSellerMustShipFirst   = errors.New("seller must mark shipped first")
	ErrMarkShippedNotAllowed = errors.New("cannot mark shipped for this auction")
	// ErrSellerClosingAuction — ผู้ขายเริ่มปิดก่อนเวลา ช่วงหน่วงไม่รับบิด
	ErrSellerClosingAuction = errors.New("bidding paused: seller is closing this auction")
	// ErrAuctionReopenNotAllowed is returned when reopen preconditions are not met.
	ErrAuctionReopenNotAllowed = errors.New("auction cannot be reopened: must be closed with no bids")
	// ErrAuctionDeleteNotAllowed is returned when delete preconditions are not met (same eligibility as reopen).
	ErrAuctionDeleteNotAllowed = errors.New("auction cannot be deleted: must be closed with no bids")
)

// sellerEarlyClosePauseDuration หลังผู้ขายกดปิดก่อนเวลา — ไม่รับบิดแล้วค่อย settle
const sellerEarlyClosePauseDuration = 3 * time.Second

// bidExtensionDuration — ทุกครั้งที่มีการบิดสำเร็จ ขยายเวลาปิดออกจาก max(end_at, now)
const bidExtensionDuration = 10 * time.Minute

const (
	maxSellerCategories = 5
	maxTitleRunes       = 255
	maxConditionRunes   = 100
	maxDescriptionRunes = 5000
)

// sellerCategoryWhitelist ต้องตรงกับตัวเลือกหมวดในแบบฟอร์มสร้างประมูล (pramool.in.th)
var sellerCategoryWhitelist = map[string]struct{}{
	"เครื่องใช้ไฟฟ้า": {},
	"โทรศัพท์มือถือ": {},
	"แท็บเล็ต":       {},
	"คอมพิวเตอร์":    {},
	"กล้องถ่ายรูป":   {},
	"แฟชั่น":        {},
	"ของสะสม":       {},
	"อื่นๆ":          {},
	"เกมคอนโซล":     {},
	"กระเป๋า":       {},
}

func normalizeSellerCategories(raw string) (string, error) {
	parts := strings.Split(raw, "|")
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, ok := sellerCategoryWhitelist[t]; !ok {
			return "", fmt.Errorf("invalid category")
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) > maxSellerCategories {
			return "", fmt.Errorf("maximum %d categories allowed", maxSellerCategories)
		}
	}
	if len(out) == 0 {
		return "", fmt.Errorf("at least one category is required")
	}
	return strings.Join(out, "|"), nil
}

type AuctionService interface {
	ListPublicAuctions(ctx context.Context, filter repository.PublicAuctionFilter) (*dto.AuctionListResponse, error)
	GetPublicUserProfile(ctx context.Context, userID string, activeLimit, reviewsLimit int) (*dto.PublicUserProfileResponse, error)
	GetAuctionDetail(ctx context.Context, auctionID string) (*dto.AuctionDetailResponse, error)
	ListAuctionBidders(ctx context.Context, auctionID string, limit int) (*dto.PublicAuctionBiddersResponse, error)
	PlaceBid(ctx context.Context, auctionID, bidderSubject string, amount int64) (*dto.PlaceBidResult, error)
	CloseAuctionEarly(ctx context.Context, auctionID, sellerID string) error
	MyActiveBids(ctx context.Context, userID, scope, q, sort string, limit, offset int) (*dto.MyActiveBidsResponse, error)
	MyBidHistory(ctx context.Context, userID string, limit, offset int) (*dto.MyBidHistoryResponse, error)
	MarkSellerShipped(ctx context.Context, auctionID, sellerUserID string) error
	ConfirmBuyerReceived(ctx context.Context, auctionID, buyerUserID string, sellerRating float64) error

	CreateSellerAuction(ctx context.Context, sellerID string, req dto.CreateAuctionRequest, imagePaths []string) (*dto.CreateAuctionResponse, error)
	ListSellerAuctions(ctx context.Context, sellerID, scope, q, sort string, limit, offset int) (*dto.SellerAuctionListResponse, error)
	ReopenSellerAuctionNoBids(ctx context.Context, sellerID, auctionID, endAtRFC3339 string) error
	DeleteSellerAuctionClosedNoBids(ctx context.Context, sellerID, auctionID string) error
}

type auctionSvc struct {
	repo                  repository.AuctionRepository
	userCredit            repository.UserCreditRepository
	hub                   *AuctionHub
	liveCache             auctionlive.Cache
	escrowAutoConfirmDays int
	platformFees          config.PlatformFeesConfig
}

// NewAuctionService constructs the auction service. escrowAutoConfirmDaysEnv is the raw value of
// ESCROW_AUTO_CONFIRM_DAYS: empty defaults to 14 calendar days after seller_shipped_at; "0" disables auto-release.
// liveCache may be auctionlive.Noop() when Redis is not configured.
func NewAuctionService(
	repo repository.AuctionRepository,
	userCredit repository.UserCreditRepository,
	hub *AuctionHub,
	escrowAutoConfirmDaysEnv string,
	liveCache auctionlive.Cache,
	platformFees config.PlatformFeesConfig,
) AuctionService {
	if liveCache == nil {
		liveCache = auctionlive.Noop()
	}
	return auctionSvc{
		repo:                  repo,
		userCredit:            userCredit,
		hub:                   hub,
		liveCache:             liveCache,
		escrowAutoConfirmDays: parseEscrowAutoConfirmDays(escrowAutoConfirmDaysEnv),
		platformFees:          platformFees,
	}
}

func (s auctionSvc) clearLiveBidderCaches(auctionID string) {
	if !s.liveCache.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.liveCache.ClearAuction(ctx, auctionID)
}

func (s auctionSvc) syncLiveBidderToRedis(ctx context.Context, auctionID string, endAt time.Time, bidderID string, amount int64) {
	if !s.liveCache.Enabled() {
		return
	}
	names, _ := s.repo.GetUserDisplayName(ctx, bidderID)
	_ = s.liveCache.UpsertBidder(ctx, auctionID, endAt, auctionlive.BidderEntry{
		BidderUserID: bidderID,
		BidAmount:    amount,
		PlacedAt:     time.Now().UTC(),
		FirstName:    names.FirstName,
		LastName:     names.LastName,
	})
}

func (s auctionSvc) biddersFromPG(ctx context.Context, auctionID string, limit int) ([]dto.PublicAuctionBidderItem, error) {
	rows, err := s.repo.ListAuctionBiddersPublic(ctx, auctionID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.PublicAuctionBidderItem, 0, len(rows))
	for _, row := range rows {
		displayName, initials := publicBidderLabels(row.FirstName, row.LastName)
		items = append(items, dto.PublicAuctionBidderItem{
			BidderUserID: row.BidderUserID,
			DisplayName:  displayName,
			Initials:     initials,
			BidAmount:    row.BidAmount,
			PlacedAt:     row.PlacedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func parseEscrowAutoConfirmDays(env string) int {
	if env == "" {
		return 14
	}
	n, err := strconv.Atoi(strings.TrimSpace(env))
	if err != nil || n < 0 {
		return 14
	}
	return n
}

func (s auctionSvc) ListPublicAuctions(ctx context.Context, filter repository.PublicAuctionFilter) (*dto.AuctionListResponse, error) {
	total, err := s.repo.CountPublicAuctions(ctx, filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListPublicAuctions(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AuctionListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.AuctionListItem{
			AuctionID:       row.AuctionID,
			Title:           row.Title,
			Category:        row.Category,
			StartPrice:      row.StartPrice,
			CurrentBid:      row.CurrentBid,
			BidStep:         row.BidStep,
			TotalBids:       row.TotalBids,
			BidderCount:     row.BidderCount,
			EndAt:           row.EndAt.Format(time.RFC3339),
			CoverImageURL:   row.CoverImageURL,
			BuyNowPrice:     row.BuyNowPrice,
			AllowEarlyClose: row.AllowEarlyClose,
		})
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	off := filter.Offset
	if off < 0 {
		off = 0
	}
	return &dto.AuctionListResponse{Items: items, Total: total, Limit: limit, Offset: off}, nil
}

func (s auctionSvc) GetPublicUserProfile(ctx context.Context, userID string, activeLimit, reviewsLimit int) (*dto.PublicUserProfileResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("missing user id")
	}
	if activeLimit <= 0 {
		activeLimit = 24
	}
	if activeLimit > 50 {
		activeLimit = 50
	}
	if reviewsLimit <= 0 {
		reviewsLimit = 50
	}
	if reviewsLimit > 100 {
		reviewsLimit = 100
	}

	profile, memberSince, err := s.repo.GetPublicUserProfileRow(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	displayName := strings.TrimSpace(profile.FirstName + " " + profile.LastName)
	if displayName == "" {
		displayName = "ผู้ใช้"
	}

	avgRating := 0.0
	if profile.ReviewCount > 0 && profile.ReviewPointsTotal > 0 {
		avgRating = float64(profile.ReviewPointsTotal) / float64(profile.ReviewCount) / 2.0
		avgRating = math.Round(avgRating*10) / 10
	}

	activeTotal, err := s.repo.CountSellerAuctionsDisplayActive(ctx, userID)
	if err != nil {
		return nil, err
	}

	auctionRows, err := s.repo.ListSellerActiveAuctionsPublic(ctx, userID, activeLimit, 0)
	if err != nil {
		return nil, err
	}
	activeItems := make([]dto.AuctionListItem, 0, len(auctionRows))
	for _, row := range auctionRows {
		activeItems = append(activeItems, dto.AuctionListItem{
			AuctionID:       row.AuctionID,
			Title:           row.Title,
			Category:        row.Category,
			StartPrice:      row.StartPrice,
			CurrentBid:      row.CurrentBid,
			BidStep:         row.BidStep,
			TotalBids:       row.TotalBids,
			BidderCount:     row.BidderCount,
			EndAt:           row.EndAt.Format(time.RFC3339),
			CoverImageURL:   row.CoverImageURL,
			BuyNowPrice:     row.BuyNowPrice,
			AllowEarlyClose: row.AllowEarlyClose,
		})
	}

	reviewRows, err := s.repo.ListSellerReviewsReceived(ctx, userID, reviewsLimit)
	if err != nil {
		return nil, err
	}
	reviews := make([]dto.PublicSellerReviewItem, 0, len(reviewRows))
	for _, row := range reviewRows {
		reviews = append(reviews, dto.PublicSellerReviewItem{
			AuctionID:    row.AuctionID,
			AuctionTitle: row.AuctionTitle,
			Rating:       row.Rating,
			CreatedAt:    row.CreatedAt.Format(time.RFC3339),
		})
	}

	return &dto.PublicUserProfileResponse{
		UserID:              userID,
		DisplayName:         displayName,
		MemberSince:         memberSince.Format(time.RFC3339),
		ReviewAvgRating:     avgRating,
		ReviewCount:         profile.ReviewCount,
		ActiveAuctions:      activeItems,
		ActiveAuctionsTotal: activeTotal,
		Reviews:             reviews,
	}, nil
}

func (s auctionSvc) ListAuctionBidders(ctx context.Context, auctionID string, limit int) (*dto.PublicAuctionBiddersResponse, error) {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return nil, fmt.Errorf("missing auction id")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	a, err := s.repo.GetAuctionByID(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("auction not found")
		}
		return nil, err
	}
	if a.Status == "closed" {
		items, err := s.biddersFromParticipants(ctx, auctionID, limit)
		if err != nil {
			return nil, err
		}
		return &dto.PublicAuctionBiddersResponse{Items: items}, nil
	}

	if s.liveCache.Enabled() {
		a, err := s.repo.GetAuctionByID(ctx, auctionID)
		if err == nil && a.Status == "active" && a.EndAt.After(time.Now()) {
			live, err := s.liveCache.ListBidders(ctx, auctionID, limit)
			if err == nil && len(live) > 0 {
				items := make([]dto.PublicAuctionBidderItem, 0, len(live))
				for _, row := range live {
					displayName, initials := publicBidderLabels(row.FirstName, row.LastName)
					items = append(items, dto.PublicAuctionBidderItem{
						BidderUserID: row.BidderUserID,
						DisplayName:  displayName,
						Initials:     initials,
						BidAmount:    row.BidAmount,
						PlacedAt:     row.PlacedAt.Format(time.RFC3339),
					})
				}
				return &dto.PublicAuctionBiddersResponse{Items: items}, nil
			}
		}
	}

	items, err := s.biddersFromPG(ctx, auctionID, limit)
	if err != nil {
		return nil, err
	}
	return &dto.PublicAuctionBiddersResponse{Items: items}, nil
}

func (s auctionSvc) biddersFromParticipants(ctx context.Context, auctionID string, limit int) ([]dto.PublicAuctionBidderItem, error) {
	rows, err := s.repo.ListAuctionBiddersFromParticipants(ctx, auctionID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.PublicAuctionBidderItem, 0, len(rows))
	for _, row := range rows {
		displayName, initials := publicBidderLabels(row.FirstName, row.LastName)
		items = append(items, dto.PublicAuctionBidderItem{
			BidderUserID: row.BidderUserID,
			DisplayName:  displayName,
			Initials:     initials,
			BidAmount:    row.BidAmount,
			PlacedAt:     row.PlacedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func mapAuctionDetailLookupErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("auction not found")
	}
	return err
}

func (s auctionSvc) GetAuctionDetail(ctx context.Context, auctionID string) (*dto.AuctionDetailResponse, error) {
	if strings.TrimSpace(auctionID) == "" {
		return nil, fmt.Errorf("auction not found")
	}

	if err := mapAuctionDetailLookupErr(s.settleAuctionIfEnded(ctx, auctionID)); err != nil {
		return nil, err
	}
	if err := mapAuctionDetailLookupErr(s.autoConfirmEscrowIfDue(ctx, auctionID)); err != nil {
		return nil, err
	}
	if err := mapAuctionDetailLookupErr(s.tryFinishSellerPauseCloseIfDue(ctx, auctionID)); err != nil {
		return nil, err
	}

	item, err := s.repo.GetAuctionByID(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("auction not found")
		}
		return nil, err
	}

	images, err := s.repo.ListAuctionImages(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	imageURLs := make([]string, 0, len(images))
	for _, img := range images {
		imageURLs = append(imageURLs, img.ImageURL)
	}
	if len(imageURLs) == 0 && strings.TrimSpace(item.CoverImageURL) != "" {
		imageURLs = append(imageURLs, item.CoverImageURL)
	}

	reopenEligible := strings.EqualFold(strings.TrimSpace(item.Status), "closed") &&
		item.TotalBids == 0 && strings.TrimSpace(item.WinnerID) == ""

	pendingSellerPayout := strings.EqualFold(strings.TrimSpace(item.Status), "closed") &&
		strings.TrimSpace(item.WinnerID) != "" && item.SellerPayoutAt == nil

	out := &dto.AuctionDetailResponse{
		AuctionID:           item.AuctionID,
		SellerID:            item.SellerID,
		WinnerID:            item.WinnerID,
		Title:               item.Title,
		Category:            item.Category,
		Condition:           item.Condition,
		Description:         item.Description,
		StartPrice:          item.StartPrice,
		CurrentBid:          item.CurrentBid,
		BidStep:             item.BidStep,
		TotalBids:           item.TotalBids,
		Status:              item.Status,
		EndAt:               item.EndAt.Format(time.RFC3339),
		AllowEarlyClose:     item.AllowEarlyClose,
		ReopenEligible:      reopenEligible,
		BuyNowPrice:         item.BuyNowPrice,
		BiddingPausedUntil:  formatTimePtr(item.SellerClosePauseBidsUntil),
		CoverImageURL:       item.CoverImageURL,
		Images:              imageURLs,
		SellerShippedAt:     formatTimePtr(item.SellerShippedAt),
		BuyerReceivedAt:     formatTimePtr(item.BuyerReceivedAt),
		SellerPayoutAt:      formatTimePtr(item.SellerPayoutAt),
		PendingSellerPayout: pendingSellerPayout,
	}
	if pendingSellerPayout && item.SellerShippedAt != nil && s.escrowAutoConfirmDays > 0 {
		out.EscrowAutoConfirmDays = s.escrowAutoConfirmDays
		t := item.SellerShippedAt.AddDate(0, 0, s.escrowAutoConfirmDays)
		out.EscrowAutoConfirmAt = t.Format(time.RFC3339)
	}
	if profile, err := s.repo.GetSellerPublicProfile(ctx, item.SellerID); err == nil {
		out.SellerDisplayName = strings.TrimSpace(profile.FirstName + " " + profile.LastName)
		out.SellerReviewCount = profile.ReviewCount
		if profile.ReviewCount > 0 && profile.ReviewPointsTotal > 0 {
			avg := float64(profile.ReviewPointsTotal) / float64(profile.ReviewCount) / 2.0
			out.SellerReviewAvgRating = math.Round(avg*10) / 10
		}
	}
	return out, nil
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (s auctionSvc) settleAuctionIfEnded(ctx context.Context, auctionID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForSettlement(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if lock.Status != "active" || lock.EndAt.After(time.Now()) {
		return tx.Commit()
	}
	if err := s.settleLockedAuction(ctx, tx, auctionID, lock, false, false); err != nil {
		return err
	}
	s.broadcastAuctionState(auctionID)
	return nil
}

func (s auctionSvc) broadcastAuctionState(auctionID string) {
	if s.hub == nil || strings.TrimSpace(auctionID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	detail, err := s.GetAuctionDetail(ctx, auctionID)
	if err != nil {
		return
	}
	re := detail.ReopenEligible
	ac := detail.AllowEarlyClose
	s.hub.Broadcast(auctionID, dto.AuctionWSMessage{
		Type:                 "auction_state",
		AuctionID:            auctionID,
		Status:               detail.Status,
		EndAt:                detail.EndAt,
		CurrentBid:           detail.CurrentBid,
		TotalBids:            detail.TotalBids,
		ReopenEligible:       &re,
		AllowEarlyClose:      &ac,
		BiddingPausedUntil:   detail.BiddingPausedUntil,
	})
}

// addSellerCredit adds delta to users.credit for listing deposit refunds (ledger type listing_deposit_refund).
func (s auctionSvc) addSellerCredit(ctx context.Context, tx bun.Tx, sellerID, auctionID string, delta int64, note string) error {
	return s.addSellerLedgerCredit(ctx, tx, sellerID, auctionID, "listing_deposit_refund", delta, note)
}

// addSellerLedgerCredit credits the seller and writes bid_transactions (generic ledger tx_type).
func (s auctionSvc) addSellerLedgerCredit(ctx context.Context, tx bun.Tx, sellerID, auctionID, txType string, delta int64, note string) error {
	if delta <= 0 {
		return nil
	}
	before, err := s.repo.LockUserCredit(ctx, tx, sellerID)
	if err != nil {
		return err
	}
	after := before + delta
	if err := s.repo.SetUserCredit(ctx, tx, sellerID, after); err != nil {
		return err
	}
	return s.repo.InsertCreditLedgerTransaction(ctx, tx, sellerID, auctionID, txType, delta, before, after, note)
}

// settleLockedAuctionCore runs settlement without committing. Caller must commit or rollback the tx.
func (s auctionSvc) settleLockedAuctionCore(ctx context.Context, tx bun.Tx, auctionID string, lock repository.AuctionSettlementLock, earlyCloseDepositConsumed bool, earlyClose bool) error {
	winnerID, _, err := s.repo.SelectWinningBidHold(ctx, tx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var refundAmount int64
			if earlyClose {
				// Close-early with no bids: return 100% of the latest visible amount to seller.
				refundAmount = lock.CurrentBid
				if refundAmount <= 0 {
					refundAmount = lock.StartPrice
				}
			} else {
				// Normal end with no bidders: refund listing deposit (start price deducted at listing in core).
				refundAmount = lock.StartPrice
			}
			note := "คืนมัดจำประกาศ (ไม่มีผู้ประมูล)"
			if earlyClose {
				note = "คืนมัดจำจากการปิดประมูลก่อนเวลา (ไม่มีผู้ประมูล)"
			}
			if err := s.addSellerCredit(ctx, tx, lock.SellerID, auctionID, refundAmount, note); err != nil {
				return err
			}
			if err := s.repo.CloseAuctionNoWinner(ctx, tx, auctionID); err != nil {
				return err
			}
			if err := s.repo.ClearAuctionBidsLive(ctx, tx, auctionID); err != nil {
				return err
			}
			if !earlyCloseDepositConsumed && lock.EarlyCloseHoldAmount > 0 {
				if err := s.repo.RefundEarlyCloseHold(ctx, tx, lock.SellerID, auctionID, lock.EarlyCloseHoldAmount); err != nil {
					return err
				}
			}
			return nil
		}
		return err
	}

	// คืนเครดิตผู้แพ้ทันที — เงินของผู้ชนะค้างใน hold สถานะ escrow จนกว่าผู้ซื้อจะกดรับของ
	losers, err := s.repo.SelectLosingBidHolds(ctx, tx, auctionID, winnerID)
	if err != nil {
		return err
	}
	for i := range losers {
		before, err := s.repo.LockUserCredit(ctx, tx, losers[i].UserID)
		if err != nil {
			return err
		}
		after := before + losers[i].HeldAmount
		if err := s.repo.SetUserCredit(ctx, tx, losers[i].UserID, after); err != nil {
			return err
		}
		if err := s.repo.InsertBidRefundTransaction(ctx, tx, losers[i].UserID, auctionID, losers[i].HeldAmount, before, after); err != nil {
			return err
		}
	}

	if err := s.repo.ReleaseNonWinningBidHolds(ctx, tx, auctionID, winnerID); err != nil {
		return err
	}
	if err := s.repo.MoveWinningHoldToEscrow(ctx, tx, auctionID, winnerID); err != nil {
		return err
	}
	if err := s.repo.CloseAuctionWithWinner(ctx, tx, auctionID, winnerID, earlyClose); err != nil {
		return err
	}
	if err := s.repo.ClearAuctionBidsLive(ctx, tx, auctionID); err != nil {
		return err
	}
	if !earlyCloseDepositConsumed && lock.EarlyCloseHoldAmount > 0 {
		if err := s.repo.RefundEarlyCloseHold(ctx, tx, lock.SellerID, auctionID, lock.EarlyCloseHoldAmount); err != nil {
			return err
		}
	}
	return nil
}

func (s auctionSvc) settleLockedAuction(ctx context.Context, tx bun.Tx, auctionID string, lock repository.AuctionSettlementLock, earlyCloseDepositConsumed bool, earlyClose bool) error {
	if err := s.settleLockedAuctionCore(ctx, tx, auctionID, lock, earlyCloseDepositConsumed, earlyClose); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.clearLiveBidderCaches(auctionID)
	return nil
}

func (s auctionSvc) CloseAuctionEarly(ctx context.Context, auctionID, sellerID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForSettlement(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if lock.Status != "active" || lock.SellerID != strings.TrimSpace(sellerID) || !lock.AllowEarlyClose {
		return ErrCannotCloseEarly
	}
	now := time.Now()
	if lock.SellerClosePauseBidsUntil != nil {
		if now.Before(*lock.SellerClosePauseBidsUntil) {
			return tx.Commit()
		}
		if err := tx.Rollback(); err != nil {
			return err
		}
		return s.tryFinishSellerPauseCloseIfDue(ctx, auctionID)
	}

	pauseUntil := now.Add(sellerEarlyClosePauseDuration)
	if err := s.repo.SetSellerClosePauseBidsUntil(ctx, tx, auctionID, pauseUntil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(auctionID)
	aid := auctionID
	d := sellerEarlyClosePauseDuration
	go func() {
		time.Sleep(d)
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = s.tryFinishSellerPauseCloseIfDue(bgCtx, aid)
	}()
	return nil
}

func (s auctionSvc) tryFinishSellerPauseCloseIfDue(ctx context.Context, auctionID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForSettlement(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	now := time.Now()
	if lock.Status != "active" {
		return tx.Commit()
	}
	if lock.SellerClosePauseBidsUntil == nil {
		return tx.Commit()
	}
	if now.Before(*lock.SellerClosePauseBidsUntil) {
		return tx.Commit()
	}
	if err := s.repo.ClearSellerClosePauseBidsUntil(ctx, tx, auctionID); err != nil {
		return err
	}
	if err := s.repo.SealAuctionBiddingEndNow(ctx, tx, auctionID); err != nil {
		return err
	}
	if lock.EarlyCloseHoldAmount > 0 {
		if err := s.repo.ZeroEarlyCloseHold(ctx, tx, auctionID); err != nil {
			return err
		}
	}
	if err := s.settleLockedAuction(ctx, tx, auctionID, lock, true, true); err != nil {
		return err
	}
	s.broadcastAuctionState(auctionID)
	return nil
}

func (s auctionSvc) PlaceBid(ctx context.Context, auctionID, bidderSubject string, amount int64) (*dto.PlaceBidResult, error) {
	if err := money.ValidatePositiveBaht(amount); err != nil {
		return nil, err
	}
	if err := s.settleAuctionIfEnded(ctx, auctionID); err != nil {
		return nil, err
	}
	if err := s.tryFinishSellerPauseCloseIfDue(ctx, auctionID); err != nil {
		return nil, err
	}
	bidder, err := s.repo.FindBidderBySubject(ctx, strings.TrimSpace(bidderSubject))
	if err != nil {
		return nil, err
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	a, err := s.repo.LockAuctionRowForUpdate(ctx, tx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("auction not found")
		}
		return nil, err
	}
	now := time.Now()
	if a.SellerID == bidder.UserID {
		return nil, ErrCannotBidOwn
	}
	if a.Status != "active" || !a.EndAt.After(now) {
		return nil, ErrAuctionClosed
	}
	if a.SellerClosePauseBidsUntil != nil && now.Before(*a.SellerClosePauseBidsUntil) {
		return nil, ErrSellerClosingAuction
	}
	if amount < a.CurrentBid+a.BidStep {
		return nil, ErrBidAmountTooLow
	}

	oldHeld, err := s.repo.SelectBidHoldForUpdate(ctx, tx, auctionID, bidder.UserID)
	if err != nil {
		return nil, err
	}
	credit, err := s.repo.LockUserCredit(ctx, tx, bidder.UserID)
	if err != nil {
		return nil, err
	}
	availableCredit := credit + oldHeld
	if availableCredit < amount {
		return nil, ErrInsufficientCredit
	}
	remainingCredit := availableCredit - amount

	if err := s.repo.SetUserCredit(ctx, tx, bidder.UserID, remainingCredit); err != nil {
		return nil, err
	}

	newEndAt := a.EndAt
	if now.After(newEndAt) {
		newEndAt = now
	}
	newEndAt = newEndAt.Add(bidExtensionDuration)

	updated, err := s.repo.UpdateAuctionOnBid(ctx, tx, auctionID, bidder.UserID, amount, newEndAt)
	if err != nil {
		if errors.Is(err, repository.ErrBidConflict) {
			return nil, ErrBidAmountTooLow
		}
		return nil, err
	}

	if err := s.repo.UpsertAuctionBidLive(ctx, tx, auctionID, bidder.UserID, amount); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertAuctionBidParticipant(ctx, tx, auctionID, bidder.UserID, amount); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertAuctionBidHold(ctx, tx, auctionID, bidder.UserID, amount); err != nil {
		return nil, err
	}
	if oldHeld != amount {
		delta := oldHeld - amount
		if err := s.repo.InsertBidHoldAdjustmentTransaction(ctx, tx, bidder.UserID, auctionID, delta, credit, remainingCredit, amount); err != nil {
			return nil, err
		}
	}

	closedByBuyNow := false
	if a.BuyNowPrice > 0 && amount >= a.BuyNowPrice {
		if err := s.repo.SealAuctionBiddingEndNow(ctx, tx, auctionID); err != nil {
			return nil, err
		}
		lock, err := s.repo.LockAuctionForSettlement(ctx, tx, auctionID)
		if err != nil {
			return nil, err
		}
		if err := s.settleLockedAuctionCore(ctx, tx, auctionID, lock, false, false); err != nil {
			return nil, err
		}
		closedByBuyNow = true
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if closedByBuyNow {
		s.clearLiveBidderCaches(auctionID)
		s.broadcastAuctionState(auctionID)
	} else {
		s.syncLiveBidderToRedis(ctx, auctionID, updated.EndAt, bidder.UserID, amount)
	}

	result := &dto.PlaceBidResult{
		AuctionID:       auctionID,
		BidderID:        bidder.UserID,
		CurrentBid:      updated.CurrentBid,
		TotalBids:       updated.TotalBids,
		RemainingCredit: remainingCredit,
		AuctionClosed:   closedByBuyNow,
	}
	if !closedByBuyNow {
		result.EndAt = updated.EndAt.Format(time.RFC3339)
	}
	return result, nil
}

func normalizeActiveBidListScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "active", "ending_soon", "outbid", "closed":
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return "all"
	}
}

func normalizeActiveBidListSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "end", "price":
		return strings.ToLower(strings.TrimSpace(sort))
	default:
		return "latest"
	}
}

func (s auctionSvc) MyActiveBids(ctx context.Context, userID, scope, q, sort string, limit, offset int) (*dto.MyActiveBidsResponse, error) {
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return &dto.MyActiveBidsResponse{Items: []dto.MyActiveBidItem{}, Limit: limit, Offset: offset, Scope: "all"}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	scope = normalizeActiveBidListScope(scope)
	sort = normalizeActiveBidListSort(sort)
	q = strings.TrimSpace(q)

	tabCounts, err := s.repo.CountMyActiveBidTabs(ctx, uid, q)
	if err != nil {
		return nil, err
	}
	scopedTotal, err := s.repo.CountMyActiveBidsScoped(ctx, uid, scope, q)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMyActiveBids(ctx, uid, scope, q, sort, limit, offset)
	if err != nil {
		return nil, err
	}
	items := make([]dto.MyActiveBidItem, 0, len(rows))
	for _, row := range rows {
		nextMin := row.CurrentBid + row.BidStep
		isLeading := strings.TrimSpace(row.LeadingUserID) == uid
		if row.CanConfirmReceived {
			isLeading = true
		}
		pauseUntil := ""
		if row.SellerClosePauseBidsUntil != nil {
			pauseUntil = row.SellerClosePauseBidsUntil.Format(time.RFC3339)
		}
		items = append(items, dto.MyActiveBidItem{
			AuctionID:          row.AuctionID,
			Title:              row.Title,
			Category:           row.Category,
			CoverImageURL:      row.CoverImageURL,
			StartPrice:         row.StartPrice,
			CurrentBid:         row.CurrentBid,
			BidStep:            row.BidStep,
			MyHeldAmount:       row.MyHeldAmount,
			NextMinimumBid:     nextMin,
			IsLeading:          isLeading,
			EndAt:              row.EndAt.Format(time.RFC3339),
			AllowEarlyClose:    row.AllowEarlyClose,
			CanConfirmReceived: row.CanConfirmReceived,
			BiddingPausedUntil: pauseUntil,
		})
	}
	return &dto.MyActiveBidsResponse{
		Items:           items,
		Total:           scopedTotal,
		AllCount:        tabCounts.All,
		ActiveCount:     tabCounts.Active,
		EndingSoonCount: tabCounts.EndingSoon,
		OutbidCount:     tabCounts.Outbid,
		ClosedCount:     tabCounts.Closed,
		Limit:           limit,
		Offset:          offset,
		Scope:           scope,
	}, nil
}

func (s auctionSvc) MyBidHistory(ctx context.Context, userID string, limit, offset int) (*dto.MyBidHistoryResponse, error) {
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return &dto.MyBidHistoryResponse{Items: []dto.MyBidHistoryItem{}, Limit: limit, Offset: offset}, nil
	}
	rows, err := s.repo.ListMyBidHistory(ctx, uid, limit, offset)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	items := make([]dto.MyBidHistoryItem, 0, len(rows))
	for _, row := range rows {
		st := strings.ToLower(strings.TrimSpace(row.Status))
		isOpen := st == "active" && row.EndAt.After(now)
		var outcome string
		switch {
		case isOpen && row.MyMaxBid < row.CurrentBid:
			outcome = "outbid"
		case isOpen:
			outcome = "active"
		case strings.TrimSpace(row.WinnerID) == uid:
			outcome = "won"
		default:
			outcome = "lost"
		}
		items = append(items, dto.MyBidHistoryItem{
			AuctionID:     row.AuctionID,
			Title:         row.Title,
			Category:      row.Category,
			CoverImageURL: row.CoverImageURL,
			Outcome:       outcome,
			AuctionStatus: strings.TrimSpace(row.Status),
			MyHighestBid:  row.MyMaxBid,
			FinalPrice:    row.CurrentBid,
			LastBidAt:     row.LastBidAt.Format(time.RFC3339),
		})
	}
	lim := limit
	if lim <= 0 {
		lim = 50
	}
	if lim > 100 {
		lim = 100
	}
	off := offset
	if off < 0 {
		off = 0
	}
	return &dto.MyBidHistoryResponse{Items: items, Limit: lim, Offset: off}, nil
}

// applySellerPayout credits the seller after escrow release (buyer confirm or auto-confirm).
func (s auctionSvc) applySellerPayout(ctx context.Context, tx bun.Tx, sellerID, auctionID, winnerUserID string, startPrice, winnerAmount int64, earlyClose, autoRelease bool) error {
	var sellerKeepPct int64
	if earlyClose {
		sellerKeepPct = s.platformFees.SellerKeepEarlyPct
	} else {
		sellerKeepPct = s.platformFees.SellerKeepNormalPct
	}
	sellerProfit, platformFee := money.SplitEscrowBySellerKeepPct(winnerAmount, sellerKeepPct)
	if platformFee > 0 {
		if err := s.repo.InsertPlatformSaleFee(ctx, tx, auctionID, sellerID, winnerUserID, winnerAmount, sellerProfit, platformFee, earlyClose, sellerKeepPct); err != nil {
			return err
		}
	}
	shareNote := "ส่วนแบ่งจากการประมูล (ยืนยันรับของแล้ว)"
	refundNote := "คืนมัดจำประกาศหลังยืนยันรับของ"
	if autoRelease {
		shareNote = "ส่วนแบ่งจากการประมูล (ปลด escrow อัตโนมัติหลังครบกำหนด)"
		refundNote = "คืนมัดจำประกาศหลังปลด escrow อัตโนมัติ"
	}
	if sellerProfit >= startPrice && sellerProfit > 0 {
		if err := s.addSellerLedgerCredit(ctx, tx, sellerID, auctionID, "seller_sale_share", sellerProfit, shareNote); err != nil {
			return err
		}
	}
	listingRefundCredit := startPrice
	if sellerProfit < startPrice {
		listingRefundCredit = sellerProfit
	}
	if listingRefundCredit > 0 {
		if err := s.addSellerCredit(ctx, tx, sellerID, auctionID, listingRefundCredit, refundNote); err != nil {
			return err
		}
	}
	return nil
}

func (s auctionSvc) releaseEscrowToSeller(ctx context.Context, tx bun.Tx, aid, winnerUserID string, lock repository.EscrowReleaseLock, autoRelease bool) error {
	winnerAmount, err := s.repo.GetWinnerEscrowHoldAmount(ctx, tx, aid, winnerUserID)
	if err != nil {
		return err
	}
	if err := s.applySellerPayout(ctx, tx, lock.SellerID, aid, winnerUserID, lock.StartPrice, winnerAmount, lock.PayoutEarlyClose, autoRelease); err != nil {
		return err
	}
	if err := s.repo.SettleWinningBidHold(ctx, tx, aid, winnerUserID); err != nil {
		return err
	}
	return s.repo.MarkAuctionDeliveryCompleted(ctx, tx, aid)
}

func (s auctionSvc) autoConfirmEscrowIfDue(ctx context.Context, auctionID string) error {
	if s.escrowAutoConfirmDays <= 0 {
		return nil
	}
	aid := strings.TrimSpace(auctionID)
	if aid == "" {
		return nil
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForEscrowRelease(ctx, tx, aid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}
	if lock.PayoutDone || !lock.SellerShipped || lock.SellerShippedAt == nil {
		return tx.Commit()
	}
	if strings.TrimSpace(lock.WinnerID) == "" {
		return tx.Commit()
	}
	deadline := lock.SellerShippedAt.AddDate(0, 0, s.escrowAutoConfirmDays)
	if time.Now().Before(deadline) {
		return tx.Commit()
	}
	if err := s.releaseEscrowToSeller(ctx, tx, aid, lock.WinnerID, lock, true); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(aid)
	return nil
}

func (s auctionSvc) MarkSellerShipped(ctx context.Context, auctionID, sellerUserID string) error {
	aid := strings.TrimSpace(auctionID)
	sid := strings.TrimSpace(sellerUserID)
	if aid == "" || sid == "" {
		return ErrMarkShippedNotAllowed
	}
	n, err := s.repo.MarkSellerShipped(ctx, aid, sid)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMarkShippedNotAllowed
	}
	return nil
}

func (s auctionSvc) ConfirmBuyerReceived(ctx context.Context, auctionID, buyerUserID string, sellerRating float64) error {
	aid := strings.TrimSpace(auctionID)
	bid := strings.TrimSpace(buyerUserID)
	if aid == "" || bid == "" {
		return ErrNotAuctionWinner
	}
	normalizedRating, sellerPoints, err := SellerPointsFromRating(sellerRating)
	if err != nil {
		return err
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForEscrowRelease(ctx, tx, aid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("auction not found")
		}
		return err
	}
	if strings.TrimSpace(lock.WinnerID) != bid {
		return ErrNotAuctionWinner
	}
	if lock.PayoutDone {
		return tx.Commit()
	}
	if !lock.SellerShipped {
		return ErrSellerMustShipFirst
	}
	hasReview, err := s.repo.HasAuctionSellerReview(ctx, tx, aid)
	if err != nil {
		return err
	}
	if !hasReview {
		if err := s.repo.InsertAuctionSellerReview(ctx, tx, aid, bid, lock.SellerID, normalizedRating, sellerPoints); err != nil {
			return err
		}
		if err := s.repo.AddSellerReviewAggregate(ctx, tx, lock.SellerID, sellerPoints); err != nil {
			return err
		}
	}
	if err := s.releaseEscrowToSeller(ctx, tx, aid, bid, lock, false); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("escrow hold not found")
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(aid)
	return nil
}

func (s auctionSvc) CreateSellerAuction(ctx context.Context, sellerID string, req dto.CreateAuctionRequest, imagePaths []string) (*dto.CreateAuctionResponse, error) {
	if strings.TrimSpace(sellerID) == "" {
		return nil, fmt.Errorf("missing seller id")
	}
	titleTrim := strings.TrimSpace(req.Title)
	if titleTrim == "" {
		return nil, fmt.Errorf("title is required")
	}
	if utf8.RuneCountInString(titleTrim) > maxTitleRunes {
		return nil, fmt.Errorf("title too long")
	}
	categoryJoined, err := normalizeSellerCategories(req.Category)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(strings.TrimSpace(req.Condition)) > maxConditionRunes {
		return nil, fmt.Errorf("condition too long")
	}
	if utf8.RuneCountInString(req.Description) > maxDescriptionRunes {
		return nil, fmt.Errorf("description too long")
	}
	if err := money.ValidatePositiveBaht(req.StartPrice); err != nil {
		return nil, fmt.Errorf("invalid start_price: %w", err)
	}
	if err := money.ValidatePositiveBaht(req.BidStep); err != nil {
		return nil, fmt.Errorf("invalid bid_step: %w", err)
	}
	if req.StartPrice < 100 {
		return nil, fmt.Errorf("invalid price settings")
	}
	if req.BuyNowPrice < 0 {
		return nil, fmt.Errorf("invalid buy_now_price")
	}
	if req.BuyNowPrice > 0 {
		if err := money.ValidatePositiveBaht(req.BuyNowPrice); err != nil {
			return nil, fmt.Errorf("invalid buy_now_price: %w", err)
		}
	}
	if req.BuyNowPrice > 0 && req.BuyNowPrice < req.StartPrice+req.BidStep {
		return nil, fmt.Errorf("buy_now_price must be at least start_price + bid_step")
	}
	if len(imagePaths) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}

	endAt, err := time.Parse(time.RFC3339, req.EndAt)
	if err != nil {
		return nil, fmt.Errorf("invalid end_at format")
	}
	if !endAt.After(time.Now()) {
		return nil, fmt.Errorf("end_at must be in the future")
	}
	auctionID := generateSellerAuctionID()

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	ok, balBefore, balAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, req.StartPrice)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !ok {
		_ = tx.Rollback()
		return nil, fmt.Errorf("insufficient credit for start price (%d THB required)", req.StartPrice)
	}

	mainAuction := entity.Auction{
		AuctionID:            auctionID,
		SellerID:             sellerID,
		Title:                titleTrim,
		Category:             categoryJoined,
		Condition:            strings.TrimSpace(req.Condition),
		Description:          strings.TrimSpace(req.Description),
		StartPrice:           req.StartPrice,
		BidStep:              req.BidStep,
		CurrentBid:           req.StartPrice,
		TotalBids:            0,
		Status:               "active",
		EndAt:                endAt,
		AllowEarlyClose:      req.AllowEarlyClose,
		EarlyCloseHoldAmount: 0,
		BuyNowPrice:          req.BuyNowPrice,
		CoverImageURL:        imagePaths[0],
	}
	if err := s.repo.CreateAuctionWithTx(ctx, tx, mainAuction); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	images := make([]entity.AuctionImage, 0, len(imagePaths))
	for i, p := range imagePaths {
		images = append(images, entity.AuctionImage{
			AuctionID: auctionID,
			ImageURL:  p,
			SortOrder: i,
		})
	}
	if err := s.repo.CreateAuctionImagesWithTx(ctx, tx, images); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := s.repo.InsertListingDepositHoldTx(ctx, tx, sellerID, auctionID, req.StartPrice, balBefore, balAfter, "หักมัดจำประกาศเมื่อสร้างประมูล"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &dto.CreateAuctionResponse{AuctionID: auctionID}, nil
}

func normalizeSellerListScope(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active", "closed":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "all"
	}
}

func normalizeSellerListSort(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "end", "price":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "latest"
	}
}

func (s auctionSvc) ListSellerAuctions(ctx context.Context, sellerID, scope, q, sort string, limit, offset int) (*dto.SellerAuctionListResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	scope = normalizeSellerListScope(scope)
	sort = normalizeSellerListSort(sort)
	q = strings.TrimSpace(q)
	allCount, err := s.repo.CountAuctionsBySellerID(ctx, sellerID)
	if err != nil {
		return nil, err
	}
	activeCount, err := s.repo.CountSellerAuctionsDisplayActive(ctx, sellerID)
	if err != nil {
		return nil, err
	}
	scopedTotal, err := s.repo.CountAuctionsBySellerIDScoped(ctx, sellerID, scope, q)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListAuctionsBySellerID(ctx, sellerID, scope, q, sort, limit, offset)
	if err != nil {
		return nil, err
	}
	auctionIDs := make([]string, 0, len(items))
	for _, item := range items {
		auctionIDs = append(auctionIDs, item.AuctionID)
	}
	reviewsByAuction, err := s.repo.ListSellerReviewsForAuctions(ctx, sellerID, auctionIDs)
	if err != nil {
		return nil, err
	}
	winnerIDs := make([]string, 0, len(items))
	for _, item := range items {
		if wid := strings.TrimSpace(item.WinnerID); wid != "" {
			winnerIDs = append(winnerIDs, wid)
		}
	}
	winnerNamesByID, err := s.repo.GetUserDisplayNamesByIDs(ctx, winnerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SellerAuctionItem, 0, len(items))
	for _, item := range items {
		reopenEligible := strings.EqualFold(strings.TrimSpace(item.Status), "closed") &&
			item.TotalBids == 0 && strings.TrimSpace(item.WinnerID) == ""
		pendingSellerPayout := strings.EqualFold(strings.TrimSpace(item.Status), "closed") &&
			strings.TrimSpace(item.WinnerID) != "" && item.SellerPayoutAt == nil
		shippedAt := ""
		if item.SellerShippedAt != nil {
			shippedAt = item.SellerShippedAt.Format(time.RFC3339)
		}
		biddingPause := ""
		if item.SellerClosePauseBidsUntil != nil {
			biddingPause = item.SellerClosePauseBidsUntil.Format(time.RFC3339)
		}
		row := dto.SellerAuctionItem{
			AuctionID:           item.AuctionID,
			Title:               item.Title,
			Category:            item.Category,
			Status:              item.Status,
			StartPrice:          item.StartPrice,
			BidStep:             item.BidStep,
			CurrentBid:          item.CurrentBid,
			TotalBids:           item.TotalBids,
			BidderCount:         item.BidderCount,
			EndAt:               item.EndAt.Format(time.RFC3339),
			CoverImageURL:       item.CoverImageURL,
			BuyNowPrice:         item.BuyNowPrice,
			AllowEarlyClose:     item.AllowEarlyClose,
			ReopenEligible:      reopenEligible,
			PendingSellerPayout: pendingSellerPayout,
			SellerShippedAt:     shippedAt,
			BiddingPausedUntil:  biddingPause,
		}
		if rev, ok := reviewsByAuction[item.AuctionID]; ok {
			row.BuyerRating = rev.Rating
			row.BuyerReviewPoints = rev.SellerPoints
		}
		if wid := strings.TrimSpace(item.WinnerID); wid != "" {
			row.WinnerID = wid
			if names, ok := winnerNamesByID[wid]; ok {
				row.WinnerDisplayName = strings.TrimSpace(names.FirstName + " " + names.LastName)
			}
			if row.WinnerDisplayName == "" {
				row.WinnerDisplayName = "ผู้ชนะ"
			}
		}
		out = append(out, row)
	}
	return &dto.SellerAuctionListResponse{
		Items:       out,
		Total:       scopedTotal,
		AllCount:    allCount,
		ActiveCount: activeCount,
		Limit:       limit,
		Offset:      offset,
		Scope:       scope,
	}, nil
}

func (s auctionSvc) ReopenSellerAuctionNoBids(ctx context.Context, sellerID, auctionID, endAtRFC3339 string) error {
	if strings.TrimSpace(sellerID) == "" || strings.TrimSpace(auctionID) == "" {
		return fmt.Errorf("missing seller or auction id")
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(endAtRFC3339))
	if err != nil {
		return fmt.Errorf("invalid end_at")
	}
	if !endAt.After(time.Now()) {
		return fmt.Errorf("end_at must be in the future")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	a, err := s.repo.LockAuctionBySellerForUpdate(ctx, tx, auctionID, sellerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("auction not found")
		}
		return err
	}
	if a.Status != "closed" || a.TotalBids != 0 || strings.TrimSpace(a.WinnerID) != "" {
		return ErrAuctionReopenNotAllowed
	}
	nBids, err := s.repo.CountAuctionBidsTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if nBids > 0 {
		return ErrAuctionReopenNotAllowed
	}
	nHeld, err := s.repo.CountHeldBidHoldsTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if nHeld > 0 {
		return ErrAuctionReopenNotAllowed
	}

	ok, balBefore, balAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, a.StartPrice)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("insufficient credit for start price (%d THB required)", a.StartPrice)
	}

	n, err := s.repo.ApplyAuctionReopenTx(ctx, tx, auctionID, sellerID, endAt)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuctionReopenNotAllowed
	}
	if err := s.repo.InsertListingDepositHoldTx(ctx, tx, sellerID, auctionID, a.StartPrice, balBefore, balAfter, "หักมัดจำประกาศเมื่อเปิดประมูลรอบใหม่"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s auctionSvc) DeleteSellerAuctionClosedNoBids(ctx context.Context, sellerID, auctionID string) error {
	if strings.TrimSpace(sellerID) == "" || strings.TrimSpace(auctionID) == "" {
		return fmt.Errorf("missing seller or auction id")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	a, err := s.repo.LockAuctionBySellerForUpdate(ctx, tx, auctionID, sellerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("auction not found")
		}
		return err
	}
	if a.Status != "closed" || a.TotalBids != 0 || strings.TrimSpace(a.WinnerID) != "" {
		return ErrAuctionDeleteNotAllowed
	}
	nBids, err := s.repo.CountAuctionBidsTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if nBids > 0 {
		return ErrAuctionDeleteNotAllowed
	}
	nHeld, err := s.repo.CountHeldBidHoldsTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if nHeld > 0 {
		return ErrAuctionDeleteNotAllowed
	}

	n, err := s.repo.DeleteClosedAuctionNoBidsTx(ctx, tx, auctionID, sellerID)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuctionDeleteNotAllowed
	}
	return tx.Commit()
}

func generateSellerAuctionID() string {
	return fmt.Sprintf("AUC-%s", strings.ToUpper(strings.ReplaceAll(time.Now().Format("20060102-150405.000"), ".", "")))
}
