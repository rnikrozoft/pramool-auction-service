package repository

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// InsertPlatformSaleFee records platform share from winner escrow (integer baht; remainder included).
func (r auctionRepo) InsertPlatformSaleFee(
	ctx context.Context,
	tx bun.Tx,
	auctionID, sellerID, winnerUserID string,
	winnerEscrow, sellerShare, platformFee int64,
	payoutEarlyClose bool,
	sellerKeepPct int64,
) error {
	if winnerEscrow <= 0 {
		return fmt.Errorf("winner escrow must be positive")
	}
	if sellerShare+platformFee != winnerEscrow {
		return fmt.Errorf("escrow split mismatch: seller %d + platform %d != winner %d", sellerShare, platformFee, winnerEscrow)
	}
	if platformFee < 0 || sellerShare < 0 {
		return fmt.Errorf("invalid split amounts")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO platform_sale_fees (
			auction_id, seller_id, winner_user_id,
			winner_escrow_amount, seller_share_amount, platform_fee_amount,
			payout_early_close, seller_keep_pct
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, auctionID, sellerID, winnerUserID, winnerEscrow, sellerShare, platformFee, payoutEarlyClose, sellerKeepPct)
	return err
}
