package repository

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func (r auctionRepo) InsertAutoRenewOptionHoldTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, holdAmount, balanceBefore, balanceAfter int64) error {
	if holdAmount <= 0 {
		return nil
	}
	note := fmt.Sprintf("หักมัดจำต่ออายุโพสอัตโนมัติ %d ฿", holdAmount)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'auto_renew_option_hold', ?, ?, ?, ?, ?)
	`, sellerID, auctionID, -holdAmount, balanceBefore, balanceAfter, note, holdAmount)
	return err
}

func (r auctionRepo) GetAutoRenewOptionHoldOutstandingTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error) {
	var held, settled int64
	err := tx.QueryRowContext(ctx, `
		SELECT
			COALESCE((SELECT SUM(bid_amount)::bigint FROM bid_transactions WHERE auction_id = ? AND tx_type = 'auto_renew_option_hold'), 0),
			COALESCE((SELECT SUM(bid_amount)::bigint FROM bid_transactions WHERE auction_id = ? AND tx_type IN (
				'auto_renew_option_refund', 'auto_renew_option_forfeit', 'auto_renew_option_consumed'
			)), 0)
	`, auctionID, auctionID).Scan(&held, &settled)
	if err != nil {
		return 0, err
	}
	out := held - settled
	if out < 0 {
		return 0, nil
	}
	return out, nil
}

func (r auctionRepo) InsertAutoRenewOptionSettlementTx(ctx context.Context, tx bun.Tx, sellerID, auctionID, txType string, amount int64, note string) error {
	if amount <= 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, ?, 0, 0, 0, ?, ?)
	`, sellerID, auctionID, txType, note, amount)
	return err
}
