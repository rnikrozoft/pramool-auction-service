package repository

import (
	"context"

	"github.com/rnikrozoft/pramool-auction-service/model/entity"
)

func (r auctionRepo) FindBidderBySubject(ctx context.Context, subject string) (*entity.Bidder, error) {
	bidder := new(entity.Bidder)
	err := r.bun.QueryRowContext(ctx, `
		SELECT user_id, COALESCE(credit, 0)
		FROM users
		WHERE user_id = ? OR tel = ?
		ORDER BY CASE WHEN user_id = ? THEN 0 ELSE 1 END
		LIMIT 1
	`, subject, subject, subject).Scan(&bidder.UserID, &bidder.Credit)
	if err != nil {
		return nil, err
	}
	return bidder, nil
}
