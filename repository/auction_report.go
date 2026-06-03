package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrReportDuplicatePending = errors.New("pending report already exists")
)

type AuctionReportSeller struct {
	SellerID string
}

func (r auctionRepo) GetAuctionSellerForReport(ctx context.Context, auctionID string) (*AuctionReportSeller, error) {
	auctionID = strings.TrimSpace(auctionID)
	row := new(AuctionReportSeller)
	err := r.bun.NewRaw(`
		SELECT seller_id
		FROM auctions
		WHERE auction_id = ?
	`, auctionID).Scan(ctx, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return row, err
}

func (r auctionRepo) HasPendingAuctionReport(ctx context.Context, auctionID, reporterUserID string) (bool, error) {
	var exists bool
	err := r.bun.NewRaw(`
		SELECT EXISTS(
			SELECT 1 FROM auction_reports
			WHERE auction_id = ? AND reporter_user_id = ? AND status = 'pending'
		)
	`, auctionID, reporterUserID).Scan(ctx, &exists)
	return exists, err
}

func (r auctionRepo) InsertAuctionReport(ctx context.Context, auctionID, sellerID, reporterUserID, reason string) (int64, error) {
	var reportID int64
	err := r.bun.NewRaw(`
		INSERT INTO auction_reports (auction_id, seller_id, reporter_user_id, reason, status)
		VALUES (?, ?, ?, ?, 'pending')
		RETURNING report_id
	`, auctionID, sellerID, reporterUserID, reason).Scan(ctx, &reportID)
	return reportID, err
}
