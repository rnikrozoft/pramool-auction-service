package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/uptrace/bun"
)

func (s auctionSvc) processFulfillmentTimeouts(ctx context.Context, auctionID string) {
	aid := strings.TrimSpace(auctionID)
	if aid == "" {
		return
	}
	_ = s.autoRefundIfSellerNoShipDue(ctx, aid)
	_ = s.autoReleaseEscrowIfDelivered(ctx, aid)
}

func (s auctionSvc) processUserFulfillmentTimeouts(ctx context.Context, userID string) {
	ids, err := s.repo.ListUserFulfillmentAuctionIDs(ctx, userID, 30)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.processFulfillmentTimeouts(ctx, id)
	}
}

func (s auctionSvc) autoRefundIfSellerNoShipDue(ctx context.Context, auctionID string) error {
	ful := s.fulfillmentConfig(ctx)
	if ful.SellerShipDeadlineDays <= 0 {
		return nil
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lock, err := s.repo.LockAuctionForNoShipRefund(ctx, tx, auctionID, ful.SellerShipDeadlineDays)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}
	winnerID := strings.TrimSpace(lock.WinnerID)
	if winnerID == "" {
		return tx.Commit()
	}
	if err := s.refundEscrowToBuyerNoShip(ctx, tx, auctionID, lock.SellerID, winnerID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.broadcastAuctionState(auctionID)
	return nil
}

func (s auctionSvc) refundEscrowToBuyerNoShip(ctx context.Context, tx bun.Tx, auctionID, sellerID, winnerID string) error {
	amount, err := s.repo.GetWinnerEscrowHoldAmount(ctx, tx, auctionID, winnerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("escrow hold not found")
		}
		return err
	}
	if amount <= 0 {
		return fmt.Errorf("invalid escrow amount")
	}
	before, err := s.repo.LockUserCredit(ctx, tx, winnerID)
	if err != nil {
		return err
	}
	after := before + amount
	if err := s.repo.SetUserCredit(ctx, tx, winnerID, after); err != nil {
		return err
	}
	note := "คืนเงินประมูล (ผู้ขายไม่จัดส่งภายในกำหนด)"
	if err := s.repo.InsertCreditLedgerTransaction(ctx, tx, winnerID, auctionID, "escrow_refund_no_ship", amount, before, after, note); err != nil {
		return err
	}
	if err := s.repo.ReleaseEscrowHoldAsRefunded(ctx, tx, auctionID, winnerID); err != nil {
		return err
	}
	if err := s.repo.MarkBuyerEscrowRefunded(ctx, tx, auctionID); err != nil {
		return err
	}
	if err := s.repo.AdjustReputationPoints(ctx, tx, sellerID, repository.SellerNoShipPenaltyPoints); err != nil {
		return err
	}
	if err := s.repo.IncrementSellerNoShipCount(ctx, tx, sellerID); err != nil {
		return err
	}
	deposit, err := s.repo.GetListingDepositHoldAmountTx(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if deposit > 0 {
		note := "ริบมัดจำประกาศ (ผู้ขายไม่จัดส่งภายในกำหนด)"
		if err := s.repo.InsertListingDepositForfeitTx(ctx, tx, sellerID, auctionID, deposit, note); err != nil {
			return err
		}
	}
	if err := s.forfeitAutoRenewOptionHoldIfAny(ctx, tx, sellerID, auctionID, "ริบมัดจำต่ออายุโพส (ผู้ขายไม่จัดส่งภายในกำหนด)"); err != nil {
		return err
	}
	if err := s.forfeitBidCancelOptionHoldIfAny(ctx, tx, sellerID, auctionID, "ริบมัดจำตัวเลือกยกเลิกบิด (ผู้ขายไม่จัดส่งภายในกำหนด)"); err != nil {
		return err
	}
	return nil
}
