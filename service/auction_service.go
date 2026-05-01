package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/repository"
)

var (
	ErrBidAmountTooLow    = errors.New("bid amount is too low")
	ErrCannotBidOwn       = errors.New("cannot bid own auction")
	ErrAuctionClosed      = errors.New("auction is closed")
	ErrInsufficientCredit = errors.New("insufficient credit")
)

type AuctionService interface {
	GetAuctionDetail(ctx context.Context, auctionID string) (*dto.AuctionDetailResponse, error)
	PlaceBid(ctx context.Context, auctionID, bidderSubject string, amount int64) (*dto.PlaceBidResult, error)
}

type auctionSvc struct {
	repo repository.AuctionRepository
}

func NewAuctionService(repo repository.AuctionRepository) AuctionService {
	return auctionSvc{repo: repo}
}

func (s auctionSvc) GetAuctionDetail(ctx context.Context, auctionID string) (*dto.AuctionDetailResponse, error) {
	if strings.TrimSpace(auctionID) == "" {
		return nil, fmt.Errorf("missing auction id")
	}

	if err := s.settleAuctionIfEnded(ctx, auctionID); err != nil {
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

	return &dto.AuctionDetailResponse{
		AuctionID:     item.AuctionID,
		SellerID:      item.SellerID,
		Title:         item.Title,
		Category:      item.Category,
		Condition:     item.Condition,
		Description:   item.Description,
		StartPrice:    item.StartPrice,
		CurrentBid:    item.CurrentBid,
		BidStep:       item.BidStep,
		TotalBids:     item.TotalBids,
		Status:        item.Status,
		EndAt:         item.EndAt.Format(time.RFC3339),
		CoverImageURL: item.CoverImageURL,
		Images:        imageURLs,
	}, nil
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

	winnerID, winnerAmount, err := s.repo.SelectWinningBidHold(ctx, tx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := s.repo.CloseAuctionNoWinner(ctx, tx, auctionID); err != nil {
				return err
			}
			return tx.Commit()
		}
		return err
	}

	sellerBefore, err := s.repo.LockUserCredit(ctx, tx, lock.SellerID)
	if err != nil {
		return err
	}
	sellerAfter := sellerBefore + winnerAmount
	if err := s.repo.SetUserCredit(ctx, tx, lock.SellerID, sellerAfter); err != nil {
		return err
	}
	if err := s.repo.InsertSellerEarningSettled(ctx, tx, lock.SellerID, auctionID, winnerID, winnerAmount); err != nil {
		return err
	}

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
	if err := s.repo.SettleWinningBidHold(ctx, tx, auctionID, winnerID); err != nil {
		return err
	}
	if err := s.repo.CloseAuctionWithWinner(ctx, tx, auctionID, winnerID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s auctionSvc) PlaceBid(ctx context.Context, auctionID, bidderSubject string, amount int64) (*dto.PlaceBidResult, error) {
	if err := s.settleAuctionIfEnded(ctx, auctionID); err != nil {
		return nil, err
	}
	a, err := s.repo.GetAuctionByID(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	bidder, err := s.repo.FindBidderBySubject(ctx, strings.TrimSpace(bidderSubject))
	if err != nil {
		return nil, err
	}
	if a.SellerID == bidder.UserID {
		return nil, ErrCannotBidOwn
	}
	if a.Status != "active" || !a.EndAt.After(time.Now()) {
		return nil, ErrAuctionClosed
	}
	if amount < a.CurrentBid+a.BidStep {
		return nil, ErrBidAmountTooLow
	}
	if amount > bidder.Credit {
		return nil, ErrInsufficientCredit
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

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

	updated, err := s.repo.UpdateAuctionOnBid(ctx, tx, auctionID, bidder.UserID, amount)
	if err != nil {
		if errors.Is(err, repository.ErrBidConflict) {
			return nil, ErrBidAmountTooLow
		}
		return nil, err
	}

	if err := s.repo.InsertAuctionBidRecord(ctx, tx, auctionID, bidder.UserID, amount); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertAuctionBidHold(ctx, tx, auctionID, bidder.UserID, amount); err != nil {
		return nil, err
	}
	if oldHeld != amount {
		delta := oldHeld - amount
		if err := s.repo.InsertBidHoldAdjustmentTransaction(ctx, tx, bidder.UserID, auctionID, delta, credit, remainingCredit); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &dto.PlaceBidResult{
		AuctionID:        auctionID,
		BidderID:         bidder.UserID,
		CurrentBid:       updated.CurrentBid,
		TotalBids:        updated.TotalBids,
		RemainingCredit:  remainingCredit,
	}, nil
}
