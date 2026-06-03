package repository

import (
	"context"
	"database/sql"

	"github.com/uptrace/bun"
)

// SellerPublicProfile is seller display name + cumulative review stats from users.
type SellerPublicProfile struct {
	FirstName                   string
	LastName                    string
	ReviewRatingPointsTotal     int64
	ReviewCount                 int
	NoShipCount                 int
	ReputationPoints            int64
}

func (r auctionRepo) GetSellerPublicProfile(ctx context.Context, sellerID string) (SellerPublicProfile, error) {
	var row SellerPublicProfile
	err := r.bun.NewRaw(`
		SELECT COALESCE(first_name, ''), COALESCE(last_name, ''),
			COALESCE(seller_review_rating_points_total, 0), COALESCE(seller_review_count, 0),
			COALESCE(seller_no_ship_count, 0),
			COALESCE(reputation_points, 0)
		FROM users
		WHERE user_id = ?
	`, sellerID).Scan(ctx, &row.FirstName, &row.LastName, &row.ReviewRatingPointsTotal, &row.ReviewCount, &row.NoShipCount, &row.ReputationPoints)
	return row, err
}

// InsertAuctionSellerReview records buyer rating before escrow release (one per auction).
func (r auctionRepo) InsertAuctionSellerReview(ctx context.Context, tx bun.Tx, auctionID, buyerUserID, sellerID string, rating float64, sellerPoints int, comment string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO auction_seller_reviews (auction_id, buyer_user_id, seller_id, rating, seller_points, comment)
		VALUES (?, ?, ?, ?, ?, ?)
	`, auctionID, buyerUserID, sellerID, rating, sellerPoints, comment)
	return err
}

// AddSellerReviewAggregate increments seller review stats and unified reputation on users row.
func (r auctionRepo) AddSellerReviewAggregate(ctx context.Context, tx bun.Tx, sellerID string, sellerPoints int) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET reputation_points = reputation_points + ?,
		    seller_review_rating_points_total = seller_review_rating_points_total + ?,
		    seller_review_count = seller_review_count + 1,
		    updated_at = NOW()
		WHERE user_id = ?
	`, sellerPoints, sellerPoints, sellerID)
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
