package repository

import (
	"context"

	"github.com/uptrace/bun"
)

// PlatformFeeSummary totals platform share recorded at seller payout (integer baht, includes split remainder).
type PlatformFeeSummary struct {
	CompletedPayoutAuctions int64 `json:"completed_payout_auctions"`

	AuctionCount25PctRule       int64 `json:"auction_count_normal_close"`
	PlatformShare25PctRuleBaht int64 `json:"platform_share_25pct_rule_baht"`

	AuctionCount30PctRule       int64 `json:"auction_count_early_close"`
	PlatformShare30PctRuleBaht int64 `json:"platform_share_30pct_rule_baht"`

	TotalPlatformShareBaht int64 `json:"total_platform_share_baht"`
}

// SummarizePlatformFeesAfterSellerPayout aggregates from platform_sale_fees (actual recorded amounts).
func SummarizePlatformFeesAfterSellerPayout(ctx context.Context, db *bun.DB) (*PlatformFeeSummary, error) {
	const q = `
SELECT
  COUNT(*)::bigint AS completed_payout_auctions,
  COALESCE(SUM(CASE WHEN NOT payout_early_close THEN 1 ELSE 0 END), 0)::bigint AS auction_count_25_rule,
  COALESCE(SUM(CASE WHEN payout_early_close THEN 1 ELSE 0 END), 0)::bigint AS auction_count_30_rule,
  COALESCE(SUM(CASE WHEN NOT payout_early_close THEN platform_fee_amount ELSE 0 END), 0)::bigint AS share_25_baht,
  COALESCE(SUM(CASE WHEN payout_early_close THEN platform_fee_amount ELSE 0 END), 0)::bigint AS share_30_baht,
  COALESCE(SUM(platform_fee_amount), 0)::bigint AS total_baht
FROM platform_sale_fees
`
	var row struct {
		Completed int64 `bun:"completed_payout_auctions"`
		C25       int64 `bun:"auction_count_25_rule"`
		C30       int64 `bun:"auction_count_30_rule"`
		S25       int64 `bun:"share_25_baht"`
		S30       int64 `bun:"share_30_baht"`
		Total     int64 `bun:"total_baht"`
	}
	if err := db.NewRaw(q).Scan(ctx, &row); err != nil {
		return nil, err
	}
	return &PlatformFeeSummary{
		CompletedPayoutAuctions:     row.Completed,
		AuctionCount25PctRule:       row.C25,
		AuctionCount30PctRule:       row.C30,
		PlatformShare25PctRuleBaht: row.S25,
		PlatformShare30PctRuleBaht: row.S30,
		TotalPlatformShareBaht:     row.Total,
	}, nil
}
