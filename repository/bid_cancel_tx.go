package repository

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func bidRefundHalf(held int64) int64 {
	return held / 2
}

func (r auctionRepo) DeleteAuctionBidLiveTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM auction_bids WHERE auction_id = ? AND bidder_user_id = ?`, auctionID, userID)
	return err
}

func (r auctionRepo) DeleteAuctionBidParticipantTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM auction_bid_participants WHERE auction_id = ? AND bidder_user_id = ?`, auctionID, userID)
	return err
}

func (r auctionRepo) DeleteAuctionBidHoldTx(ctx context.Context, tx bun.Tx, auctionID, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM auction_bid_holds WHERE auction_id = ? AND user_id = ?`, auctionID, userID)
	return err
}

func (r auctionRepo) RecalculateAuctionCurrentBidTx(ctx context.Context, tx bun.Tx, auctionID string) (int64, error) {
	var currentBid int64
	err := tx.QueryRowContext(ctx, `
		UPDATE auctions AS a
		SET current_bid = COALESCE((
			SELECT MAX(b.bid_amount) FROM auction_bids AS b WHERE b.auction_id = a.auction_id
		), a.start_price),
		    updated_at = NOW()
		WHERE a.auction_id = ?
		RETURNING current_bid
	`, auctionID).Scan(&currentBid)
	return currentBid, err
}

func (r auctionRepo) InsertBidCancelRefundTx(
	ctx context.Context,
	tx bun.Tx,
	userID, auctionID string,
	refund, forfeit, balanceBefore, balanceAfter, held int64,
) error {
	note := fmt.Sprintf("ยกเลิกการบิด คืน %d ฿ จากมัดจำ %d ฿ (ส่วนที่เหลือ %d ฿ เป็นค่าธรรมเนียม)", refund, held, forfeit)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'bid_cancel_refund', ?, ?, ?, ?, ?)
	`, userID, auctionID, refund, balanceBefore, balanceAfter, note, held)
	return err
}

func (r auctionRepo) InsertBidCancelOptionFeeTx(ctx context.Context, tx bun.Tx, sellerID, auctionID string, balanceBefore, balanceAfter int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bid_transactions (user_id, auction_id, tx_type, amount, balance_before, balance_after, note, bid_amount)
		VALUES (?, ?, 'bid_cancel_option_fee', ?, ?, ?, 'ค่าเปิดใช้ให้ผู้ประมูลยกเลิกการบิดได้', 1)
	`, sellerID, auctionID, int64(-1), balanceBefore, balanceAfter)
	return err
}
