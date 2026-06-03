package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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
	ErrCreditDebt            = errors.New("credit debt")
	ErrCannotCloseEarly      = errors.New("cannot close auction early")
	ErrNotAuctionSeller      = errors.New("not auction seller")
	ErrNotAuctionWinner      = errors.New("not auction winner")
	ErrSellerMustShipFirst   = errors.New("seller must mark shipped first")
	ErrMarkShippedNotAllowed = errors.New("cannot mark shipped for this auction")
	ErrShipmentNotDelivered    = errors.New("shipment not delivered yet")
	// ErrSellerClosingAuction — ผู้ขายเริ่มปิดก่อนเวลา ช่วงหน่วงไม่รับบิด
	ErrSellerClosingAuction = errors.New("bidding paused: seller is closing this auction")
	// ErrAuctionReopenNotAllowed is returned when reopen preconditions are not met.
	ErrAuctionReopenNotAllowed = errors.New("auction cannot be reopened: must be closed with no bids")
	// ErrAuctionDeleteNotAllowed is returned when delete preconditions are not met (same eligibility as reopen).
	ErrAuctionDeleteNotAllowed = errors.New("auction cannot be deleted: must be closed with no bids")
	ErrPostingBanned           = errors.New("account banned from creating new listings")
	ErrBidBanned               = errors.New("account banned from bidding")
	ErrCannotReportOwn         = errors.New("cannot report own auction")
	ErrBidCancelNotAllowed     = errors.New("bid cancel is not enabled for this auction")
	ErrNoBidToCancel           = errors.New("no active bid to cancel")
)

// sellerEarlyClosePauseDuration หลังผู้ขายกดปิดก่อนเวลา — ไม่รับบิดแล้วค่อย settle
const sellerEarlyClosePauseDuration = 3 * time.Second

// bidExtensionDuration — ทุกครั้งที่มีการบิดสำเร็จ หรือยกเลิกการบิด (เมื่อเปิดตัวเลือก) ขยายเวลาปิดออกจาก max(end_at, now)
const bidExtensionDuration = 10 * time.Minute

func extendAuctionEndAt(currentEnd, now time.Time) time.Time {
	newEndAt := currentEnd
	if now.After(newEndAt) {
		newEndAt = now
	}
	return newEndAt.Add(bidExtensionDuration)
}

const (
	maxSellerCategories = 5
	maxTitleRunes       = 255
	maxDescriptionRunes = 5000
)

// normalizeSellerCategories validates pipe-separated category names against product_categories.
func (s auctionSvc) normalizeSellerCategories(ctx context.Context, raw string) (string, error) {
	allowedNames, err := s.repo.ListActiveProductCategoryNames(ctx)
	if err != nil {
		return "", fmt.Errorf("load categories: %w", err)
	}
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[name] = struct{}{}
	}

	parts := strings.Split(raw, "|")
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, ok := allowed[t]; !ok {
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
	SweepFulfillmentTimeouts(ctx context.Context)
	ListPublicAuctions(ctx context.Context, filter repository.PublicAuctionFilter) (*dto.AuctionListResponse, error)
	GetPublicUserProfile(ctx context.Context, userID string, activeLimit, reviewsLimit int) (*dto.PublicUserProfileResponse, error)
	ListPublicUserClosedAuctions(ctx context.Context, userID string, limit, offset int) (*dto.AuctionListResponse, error)
	GetAuctionDetail(ctx context.Context, auctionID string) (*dto.AuctionDetailResponse, error)
	ListAuctionBidders(ctx context.Context, auctionID string, limit int) (*dto.PublicAuctionBiddersResponse, error)
	PlaceBid(ctx context.Context, auctionID, bidderSubject string, amount int64) (*dto.PlaceBidResult, error)
	CancelBid(ctx context.Context, auctionID, bidderSubject string) (*dto.CancelBidResult, error)
	CloseAuctionEarly(ctx context.Context, auctionID, sellerID string) error
	MyActiveBids(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) (*dto.MyActiveBidsResponse, error)
	MyBidHistory(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) (*dto.MyBidHistoryResponse, error)
	ConfirmBuyerReceived(ctx context.Context, auctionID, buyerUserID string, sellerRating float64, comment string) error

	CreateSellerAuction(ctx context.Context, sellerID string, req dto.CreateAuctionRequest, imagePaths []string) (*dto.CreateAuctionResponse, error)
	ListSellerAuctions(ctx context.Context, sellerID, scope, q, sort, order string, limit, offset int) (*dto.SellerAuctionListResponse, error)
	ReopenSellerAuctionNoBids(ctx context.Context, sellerID, auctionID, endAtRFC3339 string) error
	DeleteSellerAuctionClosedNoBids(ctx context.Context, sellerID, auctionID string) error
	ListProductCategories(ctx context.Context) ([]string, error)
	ReportAuction(ctx context.Context, auctionID, reporterUserID, reason string) (*dto.ReportAuctionResponse, error)
	ListingFees(ctx context.Context) dto.ListingFeesResponse
}

type auctionSvc struct {
	repo                  repository.AuctionRepository
	userCredit            repository.UserCreditRepository
	userSuspension        *repository.UserSuspensionRepository
	hub                   *AuctionHub
	liveCache             auctionlive.Cache
	policy                *PlatformPolicyLoader
}

// NewAuctionService constructs the auction service.
func NewAuctionService(
	repo repository.AuctionRepository,
	userCredit repository.UserCreditRepository,
	userSuspension *repository.UserSuspensionRepository,
	hub *AuctionHub,
	liveCache auctionlive.Cache,
	policy *PlatformPolicyLoader,
) AuctionService {
	if liveCache == nil {
		liveCache = auctionlive.Noop()
	}
	return auctionSvc{
		repo:           repo,
		userCredit:     userCredit,
		userSuspension: userSuspension,
		hub:            hub,
		liveCache:      liveCache,
		policy:         policy,
	}
}

func (s auctionSvc) SweepFulfillmentTimeouts(ctx context.Context) {
	ids, err := s.repo.ListFulfillmentSweepAuctionIDs(ctx, 100)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.processFulfillmentTimeouts(ctx, id)
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

func abbreviateSellerDisplayName(firstName, lastName string) string {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	if firstName == "" && lastName == "" {
		return "ผู้ขาย"
	}
	if lastName == "" {
		return firstName
	}
	r, _ := utf8.DecodeRuneInString(lastName)
	if firstName == "" {
		return string(r) + "."
	}
	return firstName + " " + string(r) + "."
}

// sellerAvgRatingFromAggregate converts cumulative star points (reputation_points) to 0.5–5 average.
// Penalties and admin adjustments lower the same pool as review points (rating×2 per review).
func sellerAvgRatingFromAggregate(starPointsTotal int64, reviewCount int) float64 {
	if reviewCount <= 0 {
		return 0
	}
	avg := float64(starPointsTotal) / float64(reviewCount) / 2.0
	if avg < 0 {
		avg = 0
	}
	if avg > 5 {
		avg = 5
	}
	return math.Round(avg*10) / 10
}

func publicAuctionRowToListItem(row repository.PublicAuctionRow) dto.AuctionListItem {
	item := dto.AuctionListItem{
		AuctionID:         row.AuctionID,
		Title:             row.Title,
		Category:          row.Category,
		StartPrice:        row.StartPrice,
		CurrentBid:        row.CurrentBid,
		BidStep:           row.BidStep,
		TotalBids:         row.TotalBids,
		BidderCount:       row.BidderCount,
		EndAt:             row.EndAt.Format(time.RFC3339),
		Status:            row.Status,
		CoverImageURL:     row.CoverImageURL,
		BuyNowPrice:       row.BuyNowPrice,
		AllowEarlyClose:   row.AllowEarlyClose,
		AllowBidCancel:    row.AllowBidCancel,
		SellerID:          row.SellerID,
		SellerDisplayName: abbreviateSellerDisplayName(row.SellerFirstName, row.SellerLastName),
		SellerReviewCount: row.SellerReviewCount,
	}
	item.SellerReviewAvgRating = sellerAvgRatingFromAggregate(row.SellerReputationPoints, row.SellerReviewCount)
	return item
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
		items = append(items, publicAuctionRowToListItem(row))
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

	avgRating := sellerAvgRatingFromAggregate(profile.ReputationPoints, profile.ReviewCount)

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
		item := publicAuctionRowToListItem(row)
		item.SellerID = userID
		if item.SellerDisplayName == "" || item.SellerDisplayName == "ผู้ขาย" {
			item.SellerDisplayName = abbreviateSellerDisplayName(profile.FirstName, profile.LastName)
		}
		if item.SellerReviewCount == 0 && profile.ReviewCount > 0 {
			item.SellerReviewCount = profile.ReviewCount
			item.SellerReviewAvgRating = sellerAvgRatingFromAggregate(profile.ReputationPoints, profile.ReviewCount)
		}
		activeItems = append(activeItems, item)
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
			Comment:      row.Comment,
			CreatedAt:    row.CreatedAt.Format(time.RFC3339),
		})
	}

	return &dto.PublicUserProfileResponse{
		UserID:              userID,
		DisplayName:         displayName,
		MemberSince:         memberSince.Format(time.RFC3339),
		ReviewAvgRating:     avgRating,
		ReviewCount:         profile.ReviewCount,
		SellerNoShipCount:   profile.NoShipCount,
		ActiveAuctions:      activeItems,
		ActiveAuctionsTotal: activeTotal,
		Reviews:             reviews,
	}, nil
}

func (s auctionSvc) ListPublicUserClosedAuctions(ctx context.Context, userID string, limit, offset int) (*dto.AuctionListResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("missing user id")
	}
	if limit <= 0 {
		limit = 24
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	profile, _, err := s.repo.GetPublicUserProfileRow(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	total, err := s.repo.CountSellerClosedAuctionsPublic(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.repo.ListSellerClosedAuctionsPublic(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AuctionListItem, 0, len(rows))
	for _, row := range rows {
		item := publicAuctionRowToListItem(row)
		item.SellerID = userID
		if item.SellerDisplayName == "" || item.SellerDisplayName == "ผู้ขาย" {
			item.SellerDisplayName = abbreviateSellerDisplayName(profile.FirstName, profile.LastName)
		}
		if item.SellerReviewCount == 0 && profile.ReviewCount > 0 {
			item.SellerReviewCount = profile.ReviewCount
			item.SellerReviewAvgRating = sellerAvgRatingFromAggregate(profile.ReputationPoints, profile.ReviewCount)
		}
		items = append(items, item)
	}
	return &dto.AuctionListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
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
	if err := mapAuctionDetailLookupErr(s.autoReleaseEscrowIfDelivered(ctx, auctionID)); err != nil {
		return nil, err
	}
	if err := mapAuctionDetailLookupErr(s.autoRefundIfSellerNoShipDue(ctx, auctionID)); err != nil {
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
		Description:         item.Description,
		StartPrice:          item.StartPrice,
		CurrentBid:          item.CurrentBid,
		BidStep:             item.BidStep,
		TotalBids:           item.TotalBids,
		Status:              item.Status,
		EndAt:               item.EndAt.Format(time.RFC3339),
		AllowEarlyClose:     item.AllowEarlyClose,
		AllowBidCancel:      item.AllowBidCancel,
		AutoRenew:           item.AutoRenew,
		ReopenEligible:      reopenEligible,
		BuyNowPrice:         item.BuyNowPrice,
		BiddingPausedUntil:  formatTimePtr(item.SellerClosePauseBidsUntil),
		CoverImageURL:       item.CoverImageURL,
		Images:              imageURLs,
		SellerShippedAt:     formatTimePtr(item.SellerShippedAt),
		CarrierCode:         item.CarrierCode,
		CarrierName:         item.CarrierName,
		TrackingNumber:      item.TrackingNumber,
		ShipmentStatus:      item.ShipmentStatus,
		BuyerReceivedAt:     formatTimePtr(item.BuyerReceivedAt),
		SellerPayoutAt:      formatTimePtr(item.SellerPayoutAt),
		PendingSellerPayout: pendingSellerPayout,
	}
	out.CanConfirmReceived = shipmentCanConfirm(item.ShipmentStatus, item.SellerShippedAt != nil, item.BuyerReceivedAt != nil, pendingSellerPayout)
	if profile, err := s.repo.GetSellerPublicProfile(ctx, item.SellerID); err == nil {
		out.SellerDisplayName = strings.TrimSpace(profile.FirstName + " " + profile.LastName)
		out.SellerReviewCount = profile.ReviewCount
		out.SellerReviewAvgRating = sellerAvgRatingFromAggregate(profile.ReputationPoints, profile.ReviewCount)
	}
	return out, nil
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func shipmentCanConfirm(status string, sellerShipped bool, buyerReceived bool, pendingPayout bool) bool {
	return pendingPayout && sellerShipped && !buyerReceived && strings.EqualFold(strings.TrimSpace(status), "delivered")
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
			if !earlyClose && lock.AutoRenew && lock.TotalBids == 0 {
				renewed, renewErr := s.tryAutoRenewAuctionNoBids(ctx, tx, auctionID, lock)
				if renewErr != nil {
					return renewErr
				}
				if renewed {
					return nil
				}
			}
			var refundAmount int64
			if earlyClose {
				// Close-early with no bids: return 100% of the latest visible amount to seller.
				refundAmount = lock.CurrentBid
				if refundAmount <= 0 {
					refundAmount = lock.StartPrice
				}
			} else {
				// Normal end with no bidders: refund listing deposit (10% of start price).
				refundAmount = money.ListingDepositBaht(lock.StartPrice)
			}
			note := "คืนมัดจำประกาศ (ไม่มีผู้ประมูล)"
			if earlyClose {
				note = "คืนมัดจำจากการปิดประมูลก่อนเวลา (ไม่มีผู้ประมูล)"
			}
			if err := s.addSellerCredit(ctx, tx, lock.SellerID, auctionID, refundAmount, note); err != nil {
				return err
			}
			if err := s.refundAutoRenewOptionHoldIfAny(ctx, tx, lock.SellerID, auctionID, "คืนมัดจำต่ออายุโพส (ไม่มีผู้ประมูล)"); err != nil {
				return err
			}
			if err := s.refundBidCancelOptionHoldIfAny(ctx, tx, lock.SellerID, auctionID, "คืนมัดจำตัวเลือกยกเลิกบิด (ไม่มีผู้ประมูล)"); err != nil {
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
	if err := s.ensureUserCanBid(ctx, bidderSubject); err != nil {
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
	if credit < 0 {
		return nil, ErrCreditDebt
	}
	availableCredit := credit + oldHeld
	if availableCredit < amount {
		return nil, ErrInsufficientCredit
	}
	remainingCredit := availableCredit - amount

	if err := s.repo.SetUserCredit(ctx, tx, bidder.UserID, remainingCredit); err != nil {
		return nil, err
	}

	newEndAt := extendAuctionEndAt(a.EndAt, now)

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

func (s auctionSvc) CancelBid(ctx context.Context, auctionID, bidderSubject string) (*dto.CancelBidResult, error) {
	if err := s.ensureUserCanBid(ctx, bidderSubject); err != nil {
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
	if !a.AllowBidCancel {
		return nil, ErrBidCancelNotAllowed
	}
	if a.Status != "active" || !a.EndAt.After(now) {
		return nil, ErrAuctionClosed
	}
	if a.SellerClosePauseBidsUntil != nil && now.Before(*a.SellerClosePauseBidsUntil) {
		return nil, ErrSellerClosingAuction
	}

	held, err := s.repo.SelectBidHoldForUpdate(ctx, tx, auctionID, bidder.UserID)
	if err != nil {
		return nil, err
	}
	if held <= 0 {
		return nil, ErrNoBidToCancel
	}

	refund := held / 2
	forfeit := held - refund

	credit, err := s.repo.LockUserCredit(ctx, tx, bidder.UserID)
	if err != nil {
		return nil, err
	}
	after := credit + refund
	if err := s.repo.SetUserCredit(ctx, tx, bidder.UserID, after); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteAuctionBidLiveTx(ctx, tx, auctionID, bidder.UserID); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteAuctionBidParticipantTx(ctx, tx, auctionID, bidder.UserID); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteAuctionBidHoldTx(ctx, tx, auctionID, bidder.UserID); err != nil {
		return nil, err
	}

	newCurrentBid, err := s.repo.RecalculateAuctionCurrentBidTx(ctx, tx, auctionID)
	if err != nil {
		return nil, err
	}

	newEndAt := extendAuctionEndAt(a.EndAt, now)
	if err := s.repo.UpdateAuctionEndAtTx(ctx, tx, auctionID, newEndAt); err != nil {
		return nil, err
	}

	if err := s.repo.InsertBidCancelRefundTx(ctx, tx, bidder.UserID, auctionID, refund, forfeit, credit, after, held); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	s.removeLiveBidderFromCache(ctx, auctionID, bidder.UserID)
	s.broadcastAuctionState(auctionID)

	return &dto.CancelBidResult{
		AuctionID:       auctionID,
		RefundedBaht:    refund,
		ForfeitedBaht:   forfeit,
		RemainingCredit: after,
		CurrentBid:      newCurrentBid,
		EndAt:           newEndAt.Format(time.RFC3339),
	}, nil
}

func (s auctionSvc) removeLiveBidderFromCache(ctx context.Context, auctionID, bidderID string) {
	if !s.liveCache.Enabled() {
		return
	}
	_ = s.liveCache.RemoveBidder(ctx, auctionID, bidderID)
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
	case "end", "price", "start", "step", "my_bid", "status":
		return strings.ToLower(strings.TrimSpace(sort))
	default:
		return "latest"
	}
}

func (s auctionSvc) MyActiveBids(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) (*dto.MyActiveBidsResponse, error) {
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return &dto.MyActiveBidsResponse{Items: []dto.MyActiveBidItem{}, Limit: limit, Offset: offset, Scope: "all"}, nil
	}
	s.processUserFulfillmentTimeouts(ctx, uid)
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
	rows, err := s.repo.ListMyActiveBids(ctx, uid, scope, q, sort, order, limit, offset)
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
			ShipmentStatus:     row.ShipmentStatus,
			BiddingPausedUntil: pauseUntil,
			CreatedAt:          row.CreatedAt.Format(time.RFC3339),
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

func normalizeBidHistoryScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "active", "outbid", "won", "lost":
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return "all"
	}
}

func normalizeBidHistorySort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "price", "my_bid", "status", "end":
		return strings.ToLower(strings.TrimSpace(sort))
	default:
		return "latest"
	}
}

func (s auctionSvc) MyBidHistory(ctx context.Context, userID, scope, q, sort, order string, limit, offset int) (*dto.MyBidHistoryResponse, error) {
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return &dto.MyBidHistoryResponse{Items: []dto.MyBidHistoryItem{}, Limit: limit, Offset: offset, Scope: "all"}, nil
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
	scope = normalizeBidHistoryScope(scope)
	sort = normalizeBidHistorySort(sort)
	q = strings.TrimSpace(q)

	tabCounts, err := s.repo.CountMyBidHistoryTabs(ctx, uid, q)
	if err != nil {
		return nil, err
	}
	scopedTotal, err := s.repo.CountMyBidHistoryScoped(ctx, uid, scope, q)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMyBidHistory(ctx, uid, scope, q, sort, order, limit, offset)
	if err != nil {
		return nil, err
	}
	items := make([]dto.MyBidHistoryItem, 0, len(rows))
	for _, row := range rows {
		outcome := strings.ToLower(strings.TrimSpace(row.Outcome))
		if outcome != "active" && outcome != "outbid" && outcome != "won" && outcome != "lost" {
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
			EndAt:         row.EndAt.Format(time.RFC3339),
		})
	}
	return &dto.MyBidHistoryResponse{
		Items:       items,
		Total:       scopedTotal,
		AllCount:    tabCounts.All,
		ActiveCount: tabCounts.Active,
		OutbidCount: tabCounts.Outbid,
		WonCount:    tabCounts.Won,
		LostCount:   tabCounts.Lost,
		Limit:       limit,
		Offset:      offset,
		Scope:       scope,
	}, nil
}

// applySellerPayout credits the seller after escrow release (buyer confirm or auto-confirm).
func (s auctionSvc) applySellerPayout(ctx context.Context, tx bun.Tx, sellerID, auctionID, winnerUserID string, startPrice, winnerAmount int64, createdAt, endAt time.Time, earlyClose, autoRenew, autoRelease bool) error {
	listingCfg := s.listingFeesConfig(ctx)
	extraDays := extraListingDaysBeyondFree(listingCfg, createdAt, endAt)
	durationFee := sellerListingDurationFeeBaht(listingCfg, winnerAmount, extraDays)
	var sellerKeepPct int64
	if earlyClose {
		sellerKeepPct = s.platformFeesConfig(ctx).SellerKeepEarlyPct
	} else {
		sellerKeepPct = s.platformFeesConfig(ctx).SellerKeepNormalPct
	}
	sellerProfit, platformSaleFee := money.SplitEscrowBySellerKeepPct(winnerAmount, sellerKeepPct)
	extraSellerFee := durationFee
	if extraSellerFee > sellerProfit {
		extraSellerFee = sellerProfit
	}
	sellerProfit -= extraSellerFee
	platformFee := platformSaleFee + extraSellerFee
	if platformFee > 0 || sellerProfit > 0 {
		if err := s.repo.InsertPlatformSaleFee(ctx, tx, auctionID, sellerID, winnerUserID, winnerAmount, sellerProfit, platformFee, earlyClose, sellerKeepPct); err != nil {
			return err
		}
	}
	shareNote := "ส่วนแบ่งจากการประมูล (พัสดุส่งถึงแล้ว)"
	refundNote := "คืนมัดจำประกาศหลังพัสดุส่งถึง"
	if autoRelease {
		shareNote = "ส่วนแบ่งจากการประมูล (ระบบปลด escrow อัตโนมัติเมื่อพัสดุ delivered)"
		refundNote = "คืนมัดจำประกาศหลังพัสดุส่งถึง"
	}
	if durationFee > 0 {
		shareNote = fmt.Sprintf("%s · หักค่าประมูลเกิน %d วัน %d ฿ จากส่วนแบ่งผู้ขาย", shareNote, listingCfg.FreeListingDurationDays, durationFee)
	}
	if sellerProfit > 0 {
		if err := s.addSellerLedgerCredit(ctx, tx, sellerID, auctionID, "seller_sale_share", sellerProfit, shareNote); err != nil {
			return err
		}
	}
	deposit, err := s.repo.GetListingDepositHoldAmountTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if deposit <= 0 {
		deposit = money.ListingDepositBaht(startPrice)
	}
	if deposit > 0 {
		if err := s.addSellerCredit(ctx, tx, sellerID, auctionID, deposit, refundNote); err != nil {
			return err
		}
	}
	if err := s.consumeAutoRenewOptionHoldIfAny(ctx, tx, sellerID, auctionID, "มัดจำต่ออายุโพส — มีผู้ชนะ (เข้าแพลตฟอร์ม)"); err != nil {
		return err
	}
	if err := s.consumeBidCancelOptionHoldIfAny(ctx, tx, sellerID, auctionID, "มัดจำตัวเลือกยกเลิกบิด — มีผู้ชนะ (เข้าแพลตฟอร์ม)"); err != nil {
		return err
	}
	return nil
}

func (s auctionSvc) tryAutoRenewAuctionNoBids(ctx context.Context, tx bun.Tx, auctionID string, lock repository.AuctionSettlementLock) (bool, error) {
	if !lock.AutoRenew {
		return false, nil
	}
	duration := lock.EndAt.Sub(lock.CreatedAt)
	if duration < time.Minute {
		duration = maxListingDurationFromCfg(s.listingFeesConfig(ctx))
	}
	newEndAt := time.Now().Add(duration)

	n, err := s.repo.ApplyAuctionAutoRenewTx(ctx, tx, auctionID, lock.SellerID, newEndAt)
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, nil
	}
	if err := s.repo.ClearAuctionBidsLive(ctx, tx, auctionID); err != nil {
		return false, err
	}
	return true, nil
}

func (s auctionSvc) releaseEscrowToSeller(ctx context.Context, tx bun.Tx, aid, winnerUserID string, lock repository.EscrowReleaseLock, autoRelease, sellerPayoutOnly bool) error {
	winnerAmount, err := s.repo.GetWinnerEscrowHoldAmount(ctx, tx, aid, winnerUserID)
	if err != nil {
		return err
	}
	if err := s.applySellerPayout(ctx, tx, lock.SellerID, aid, winnerUserID, lock.StartPrice, winnerAmount, lock.CreatedAt, lock.EndAt, lock.PayoutEarlyClose, lock.AutoRenew, autoRelease); err != nil {
		return err
	}
	if err := s.repo.SettleWinningBidHold(ctx, tx, aid, winnerUserID); err != nil {
		return err
	}
	if sellerPayoutOnly {
		return s.repo.MarkSellerPayoutOnly(ctx, tx, aid)
	}
	return s.repo.MarkAuctionDeliveryCompleted(ctx, tx, aid)
}

func (s auctionSvc) autoReleaseEscrowIfDelivered(ctx context.Context, auctionID string) error {
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
	if lock.PayoutDone || !lock.SellerShipped {
		return tx.Commit()
	}
	if strings.TrimSpace(lock.WinnerID) == "" {
		return tx.Commit()
	}
	if !strings.EqualFold(strings.TrimSpace(lock.ShipmentStatus), "delivered") {
		return tx.Commit()
	}
	if err := s.releaseEscrowToSeller(ctx, tx, aid, lock.WinnerID, lock, true, true); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(aid)
	return nil
}

func (s auctionSvc) ConfirmBuyerReceived(ctx context.Context, auctionID, buyerUserID string, sellerRating float64, comment string) error {
	aid := strings.TrimSpace(auctionID)
	bid := strings.TrimSpace(buyerUserID)
	if aid == "" || bid == "" {
		return ErrNotAuctionWinner
	}
	normalizedRating, sellerPoints, err := SellerPointsFromRating(sellerRating)
	if err != nil {
		return err
	}
	comment = normalizeReviewComment(comment)
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
	if lock.BuyerReceivedDone {
		return tx.Commit()
	}
	if !lock.SellerShipped {
		return ErrSellerMustShipFirst
	}
	if !strings.EqualFold(strings.TrimSpace(lock.ShipmentStatus), "delivered") {
		return ErrShipmentNotDelivered
	}
	if !lock.PayoutDone {
		if err := s.releaseEscrowToSeller(ctx, tx, aid, bid, lock, true, true); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("escrow hold not found")
			}
			return err
		}
	}
	hasReview, err := s.repo.HasAuctionSellerReview(ctx, tx, aid)
	if err != nil {
		return err
	}
	if !hasReview {
		if err := s.repo.InsertAuctionSellerReview(ctx, tx, aid, bid, lock.SellerID, normalizedRating, sellerPoints, comment); err != nil {
			return err
		}
		if err := s.repo.AddSellerReviewAggregate(ctx, tx, lock.SellerID, sellerPoints); err != nil {
			return err
		}
	}
	if err := s.repo.MarkBuyerReceivedOnly(ctx, tx, aid); err != nil {
		return err
	}
	if err := s.repo.AdjustReputationPoints(ctx, tx, bid, repository.BuyerConfirmReviewRewardPoints); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(aid)
	return nil
}

func (s auctionSvc) ensureUserCanBid(ctx context.Context, subject string) error {
	if s.userSuspension == nil {
		return nil
	}
	banned, err := s.userSuspension.IsUserBanned(ctx, subject)
	if err != nil {
		return err
	}
	if banned {
		return ErrBidBanned
	}
	return nil
}

func (s auctionSvc) ensureSellerCanPost(ctx context.Context, sellerID string) error {
	if s.userSuspension == nil {
		return nil
	}
	banned, err := s.userSuspension.IsUserPostingBanned(ctx, sellerID)
	if err != nil {
		return err
	}
	if banned {
		return ErrPostingBanned
	}
	return nil
}

func (s auctionSvc) CreateSellerAuction(ctx context.Context, sellerID string, req dto.CreateAuctionRequest, imagePaths []string) (*dto.CreateAuctionResponse, error) {
	if strings.TrimSpace(sellerID) == "" {
		return nil, fmt.Errorf("missing seller id")
	}
	if err := s.ensureSellerCanPost(ctx, sellerID); err != nil {
		return nil, err
	}
	titleTrim := strings.TrimSpace(req.Title)
	if titleTrim == "" {
		return nil, fmt.Errorf("title is required")
	}
	if utf8.RuneCountInString(titleTrim) > maxTitleRunes {
		return nil, fmt.Errorf("title too long")
	}
	categoryJoined, err := s.normalizeSellerCategories(ctx, req.Category)
	if err != nil {
		return nil, err
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
	listingCfg := s.listingFeesConfig(ctx)
	if req.StartPrice < listingCfg.MinStartPriceTHB {
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
	if err := validateAuctionEndAt(endAt); err != nil {
		return nil, err
	}
	auctionID := generateSellerAuctionID()
	listingDeposit := money.ListingDepositBaht(req.StartPrice)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	ok, balBefore, balAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, listingDeposit)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !ok {
		_ = tx.Rollback()
		msg := fmt.Sprintf("insufficient credit for listing deposit (%d THB required, 10%% of start price)", listingDeposit)
		if req.AllowBidCancel {
			msg = fmt.Sprintf("insufficient credit (%d THB deposit + %d THB bid-cancel option required)", listingDeposit, listingCfg.BidCancelOptionFeeTHB)
		}
		if req.AutoRenew {
			msg = fmt.Sprintf("insufficient credit (%d THB deposit + %d THB auto-renew option required)", listingDeposit, listingCfg.AutoRenewOptionFeeTHB)
			if req.AllowBidCancel {
				msg = fmt.Sprintf("insufficient credit (%d THB deposit + %d THB bid-cancel + %d THB auto-renew required)", listingDeposit, listingCfg.BidCancelOptionFeeTHB, listingCfg.AutoRenewOptionFeeTHB)
			}
		}
		return nil, errors.New(msg)
	}

	var bidCancelFeeBefore, bidCancelFeeAfter int64
	if req.AllowBidCancel {
		okFee, feeBefore, feeAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, listingCfg.BidCancelOptionFeeTHB)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if !okFee {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insufficient credit for bid-cancel option (%d THB required)", listingCfg.BidCancelOptionFeeTHB)
		}
		bidCancelFeeBefore, bidCancelFeeAfter = feeBefore, feeAfter
	}

	mainAuction := entity.Auction{
		AuctionID:            auctionID,
		SellerID:             sellerID,
		Title:                titleTrim,
		Category:             categoryJoined,
		Description:          strings.TrimSpace(req.Description),
		StartPrice:           req.StartPrice,
		BidStep:              req.BidStep,
		CurrentBid:           req.StartPrice,
		TotalBids:            0,
		Status:               "active",
		EndAt:                endAt,
		AllowEarlyClose:      req.AllowEarlyClose,
		AllowBidCancel:       req.AllowBidCancel,
		AutoRenew:            req.AutoRenew,
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

	if err := s.repo.InsertListingDepositHoldTx(ctx, tx, sellerID, auctionID, listingDeposit, balBefore, balAfter, "หักมัดจำประกาศ 10% เมื่อสร้างประมูล"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if req.AllowBidCancel {
		bidCancelFee := listingCfg.BidCancelOptionFeeTHB
		if err := s.repo.InsertBidCancelOptionHoldTx(ctx, tx, sellerID, auctionID, bidCancelFee, bidCancelFeeBefore, bidCancelFeeAfter); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if req.AutoRenew {
		autoRenewFee := listingCfg.AutoRenewOptionFeeTHB
		okRenew, renewBefore, renewAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, autoRenewFee)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if !okRenew {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insufficient credit for auto-renew option (%d THB required)", autoRenewFee)
		}
		if err := s.repo.InsertAutoRenewOptionHoldTx(ctx, tx, sellerID, auctionID, autoRenewFee, renewBefore, renewAfter); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
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
	case "end", "price", "start", "step", "bidders", "status":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "latest"
	}
}

func (s auctionSvc) ListSellerAuctions(ctx context.Context, sellerID, scope, q, sort, order string, limit, offset int) (*dto.SellerAuctionListResponse, error) {
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
	items, err := s.repo.ListAuctionsBySellerID(ctx, sellerID, scope, q, sort, order, limit, offset)
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
			AllowBidCancel:      item.AllowBidCancel,
			AutoRenew:           item.AutoRenew,
			ReopenEligible:      reopenEligible,
			PendingSellerPayout: pendingSellerPayout,
			SellerShippedAt:     shippedAt,
			BiddingPausedUntil:  biddingPause,
			CreatedAt:           item.CreatedAt.Format(time.RFC3339),
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
	if err := s.ensureSellerCanPost(ctx, sellerID); err != nil {
		return err
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(endAtRFC3339))
	if err != nil {
		return fmt.Errorf("invalid end_at")
	}
	if !endAt.After(time.Now()) {
		return fmt.Errorf("end_at must be in the future")
	}
	if err := validateAuctionEndAt(endAt); err != nil {
		return err
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

	listingDeposit := money.ListingDepositBaht(a.StartPrice)
	ok, balBefore, balAfter, err := s.userCredit.DeductListingDepositTx(ctx, tx, sellerID, listingDeposit)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("insufficient credit for listing deposit (%d THB required, 10%% of start price)", listingDeposit)
	}

	n, err := s.repo.ApplyAuctionReopenTx(ctx, tx, auctionID, sellerID, endAt)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuctionReopenNotAllowed
	}
	if err := s.repo.InsertListingDepositHoldTx(ctx, tx, sellerID, auctionID, listingDeposit, balBefore, balAfter, "หักมัดจำประกาศ 10% เมื่อเปิดประมูลรอบใหม่"); err != nil {
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
	if err := s.refundAutoRenewOptionHoldIfAny(ctx, tx, sellerID, auctionID, "คืนมัดจำต่ออายุโพส (ลบรายการไม่มีผู้บิด)"); err != nil {
		return err
	}
	if err := s.refundBidCancelOptionHoldIfAny(ctx, tx, sellerID, auctionID, "คืนมัดจำตัวเลือกยกเลิกบิด (ลบรายการไม่มีผู้บิด)"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s auctionSvc) ListProductCategories(ctx context.Context) ([]string, error) {
	return s.repo.ListActiveProductCategoryNames(ctx)
}

func (s auctionSvc) ReportAuction(ctx context.Context, auctionID, reporterUserID, reason string) (*dto.ReportAuctionResponse, error) {
	auctionID = strings.TrimSpace(auctionID)
	reporterUserID = strings.TrimSpace(reporterUserID)
	reason = strings.TrimSpace(reason)
	if auctionID == "" || reporterUserID == "" {
		return nil, fmt.Errorf("invalid request")
	}
	if utf8.RuneCountInString(reason) > 2000 {
		return nil, fmt.Errorf("reason too long")
	}

	seller, err := s.repo.GetAuctionSellerForReport(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("auction not found")
		}
		return nil, err
	}
	if seller.SellerID == reporterUserID {
		return nil, ErrCannotReportOwn
	}

	dup, err := s.repo.HasPendingAuctionReport(ctx, auctionID, reporterUserID)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, repository.ErrReportDuplicatePending
	}

	reportID, err := s.repo.InsertAuctionReport(ctx, auctionID, seller.SellerID, reporterUserID, reason)
	if err != nil {
		return nil, err
	}
	return &dto.ReportAuctionResponse{
		ReportID: reportID,
		Message:  "ส่งเรื่องร้องเรียนแล้ว ทีมงานจะตรวจสอบ",
	}, nil
}

func (s auctionSvc) listingFeesConfig(ctx context.Context) config.ListingFeesConfig {
	return s.policy.Get(ctx).Listing
}

func (s auctionSvc) platformFeesConfig(ctx context.Context) config.PlatformFeesConfig {
	return s.policy.Get(ctx).PlatformFees
}

func (s auctionSvc) fulfillmentConfig(ctx context.Context) config.FulfillmentConfig {
	return s.policy.Get(ctx).Fulfillment
}

func (s auctionSvc) ListingFees(ctx context.Context) dto.ListingFeesResponse {
	cfg := s.listingFeesConfig(ctx)
	return dto.ListingFeesResponse{
		MinStartPriceTHB:        cfg.MinStartPriceTHB,
		BidCancelOptionFeeTHB:   cfg.BidCancelOptionFeeTHB,
		FreeListingDurationDays: cfg.FreeListingDurationDays,
		ExtraListingDayFeePct:   cfg.ExtraListingDayFeePct,
		AutoRenewOptionFeeTHB:   cfg.AutoRenewOptionFeeTHB,
	}
}

func generateSellerAuctionID() string {
	return fmt.Sprintf("AUC-%s", strings.ToUpper(strings.ReplaceAll(time.Now().Format("20060102-150405.000"), ".", "")))
}
