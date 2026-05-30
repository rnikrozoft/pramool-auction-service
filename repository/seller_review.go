package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/uptrace/bun"
)

// SellerPublicProfile is seller display name + cumulative review stats from users.
type SellerPublicProfile struct {
	FirstName         string
	LastName          string
	ReviewPointsTotal int64
	ReviewCount       int
}

func (r auctionRepo) GetSellerPublicProfile(ctx context.Context, sellerID string) (SellerPublicProfile, error) {
	var row SellerPublicProfile
	err := r.bun.NewRaw(`
		SELECT COALESCE(first_name, ''), COALESCE(last_name, ''),
			COALESCE(seller_review_points_total, 0), COALESCE(seller_review_count, 0)
		FROM users
		WHERE user_id = ?
	`, sellerID).Scan(ctx, &row.FirstName, &row.LastName, &row.ReviewPointsTotal, &row.ReviewCount)
	return row, err
}

// SellerAuctionReviewRow is a buyer review for one auction (seller list).
type SellerAuctionReviewRow struct {
	AuctionID    string  `bun:"auction_id"`
	Rating       float64 `bun:"rating"`
	SellerPoints int     `bun:"seller_points"`
}

// InsertAuctionSellerReview records buyer rating before escrow release (one per auction).
func (r auctionRepo) InsertAuctionSellerReview(ctx context.Context, tx bun.Tx, auctionID, buyerUserID, sellerID string, rating float64, sellerPoints int) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_seller_reviews (auction_id, buyer_user_id, seller_id, rating, seller_points)
		VALUES (?, ?, ?, ?, ?)
	`, auctionID, buyerUserID, sellerID, rating, sellerPoints)
	return err
}

// AddSellerReviewAggregate increments seller's cumulative review points on users row.
func (r auctionRepo) AddSellerReviewAggregate(ctx context.Context, tx bun.Tx, sellerID string, sellerPoints int) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET seller_review_points_total = seller_review_points_total + ?,
		    seller_review_count = seller_review_count + 1,
		    updated_at = NOW()
		WHERE user_id = ?
	`, sellerPoints, sellerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// HasAuctionSellerReview returns true if buyer already rated this auction.
func (r auctionRepo) HasAuctionSellerReview(ctx context.Context, tx bun.Tx, auctionID string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)::int FROM auction_seller_reviews WHERE auction_id = ?
	`, auctionID).Scan(&n)
	return n > 0, err
}

// ListSellerReviewsForAuctions returns buyer reviews keyed by auction_id for the seller's listings.
func (r auctionRepo) ListSellerReviewsForAuctions(ctx context.Context, sellerID string, auctionIDs []string) (map[string]SellerAuctionReviewRow, error) {
	out := make(map[string]SellerAuctionReviewRow)
	if strings.TrimSpace(sellerID) == "" || len(auctionIDs) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(auctionIDs))
	for _, id := range auctionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, sellerID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := `
		SELECT auction_id, rating::float8 AS rating, seller_points
		FROM auction_seller_reviews
		WHERE seller_id = ? AND auction_id IN (` + strings.Join(placeholders, ",") + `)
	`
	var rows []SellerAuctionReviewRow
	if err := r.bun.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AuctionID] = row
	}
	return out, nil
}
