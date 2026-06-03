package service

import (
	"context"

	"github.com/uptrace/bun"
)

func (s auctionSvc) refundAutoRenewOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetAutoRenewOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.addSellerLedgerCredit(ctx, tx, sellerID, auctionID, "auto_renew_option_refund", outstanding, note)
}

func (s auctionSvc) consumeAutoRenewOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetAutoRenewOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.repo.InsertAutoRenewOptionSettlementTx(ctx, tx, sellerID, auctionID, "auto_renew_option_consumed", outstanding, note)
}

func (s auctionSvc) forfeitAutoRenewOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetAutoRenewOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.repo.InsertAutoRenewOptionSettlementTx(ctx, tx, sellerID, auctionID, "auto_renew_option_forfeit", outstanding, note)
}
