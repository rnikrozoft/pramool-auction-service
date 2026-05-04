package repository

import (
	"context"

	"github.com/uptrace/bun"
)

// PlatformFeeSummary totals implied platform share (25% or 30% of winner escrow) after seller payout.
type PlatformFeeSummary struct {
	CompletedPayoutAuctions int64 `json:"completed_payout_auctions"`

	AuctionCount25PctRule       int64 `json:"auction_count_normal_close"`
	PlatformShare25PctRuleMinor int64 `json:"platform_share_25pct_rule_minor"`

	AuctionCount30PctRule       int64 `json:"auction_count_early_close"`
	PlatformShare30PctRuleMinor int64 `json:"platform_share_30pct_rule_minor"`

	TotalPlatformShareMinor int64 `json:"total_platform_share_minor"`
}

// SummarizePlatformFeesAfterSellerPayout aggregates for auctions with seller_payout_at set.
func SummarizePlatformFeesAfterSellerPayout(ctx context.Context, db *bun.DB) (*PlatformFeeSummary, error) {
	const q = `
WITH base AS (
  SELECT
    a.auction_id,
    a.payout_early_close,
    COALESCE(
      (
        SELECT h.held_amount
        FROM auction_bid_holds h
        WHERE h.auction_id = a.auction_id
          AND h.user_id = a.winner_id
          AND h.hold_status = 'settled'
        ORDER BY h.held_amount DESC
        LIMIT 1
      ),
      a.current_bid
    ) AS winner_base
  FROM auctions a
  WHERE a.status = 'closed'
    AND NULLIF(TRIM(a.winner_id), '') IS NOT NULL
    AND a.seller_payout_at IS NOT NULL
)
SELECT
  COUNT(*)::bigint AS completed_payout_auctions,
  COALESCE(SUM(CASE WHEN NOT payout_early_close THEN 1 ELSE 0 END), 0)::bigint AS auction_count_25_rule,
  COALESCE(SUM(CASE WHEN payout_early_close THEN 1 ELSE 0 END), 0)::bigint AS auction_count_30_rule,
  COALESCE(SUM(CASE WHEN NOT payout_early_close THEN winner_base * 25 / 100 ELSE 0 END), 0)::bigint AS share_25_minor,
  COALESCE(SUM(CASE WHEN payout_early_close THEN winner_base * 30 / 100 ELSE 0 END), 0)::bigint AS share_30_minor,
  COALESCE(SUM(
    CASE
      WHEN payout_early_close THEN winner_base * 30 / 100
      ELSE winner_base * 25 / 100
    END
  ), 0)::bigint AS total_minor
FROM base
`
	var row struct {
		Completed int64 `bun:"completed_payout_auctions"`
		C25       int64 `bun:"auction_count_25_rule"`
		C30       int64 `bun:"auction_count_30_rule"`
		S25       int64 `bun:"share_25_minor"`
		S30       int64 `bun:"share_30_minor"`
		Total     int64 `bun:"total_minor"`
	}
	if err := db.NewRaw(q).Scan(ctx, &row); err != nil {
		return nil, err
	}
	return &PlatformFeeSummary{
		CompletedPayoutAuctions:     row.Completed,
		AuctionCount25PctRule:       row.C25,
		AuctionCount30PctRule:       row.C30,
		PlatformShare25PctRuleMinor: row.S25,
		PlatformShare30PctRuleMinor: row.S30,
		TotalPlatformShareMinor:     row.Total,
	}, nil
}
