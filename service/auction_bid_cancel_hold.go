package service

import (
	"context"

	"github.com/uptrace/bun"
)

func (s auctionSvc) refundBidCancelOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetBidCancelOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.addSellerLedgerCredit(ctx, tx, sellerID, auctionID, "bid_cancel_option_refund", outstanding, note)
}

func (s auctionSvc) consumeBidCancelOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetBidCancelOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.repo.InsertBidCancelOptionSettlementTx(ctx, tx, sellerID, auctionID, "bid_cancel_option_consumed", outstanding, note)
}

func (s auctionSvc) forfeitBidCancelOptionHoldIfAny(ctx context.Context, tx bun.Tx, sellerID, auctionID, note string) error {
	outstanding, err := s.repo.GetBidCancelOptionHoldOutstandingTx(ctx, tx, auctionID)
	if err != nil || outstanding <= 0 {
		return err
	}
	return s.repo.InsertBidCancelOptionSettlementTx(ctx, tx, sellerID, auctionID, "bid_cancel_option_forfeit", outstanding, note)
}
