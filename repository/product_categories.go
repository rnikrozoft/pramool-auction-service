package repository

import "context"

func (r auctionRepo) ListActiveProductCategoryNames(ctx context.Context) ([]string, error) {
	var names []string
	err := r.bun.NewRaw(`
		SELECT name
		FROM product_categories
		WHERE is_active = TRUE OR is_system = TRUE
		ORDER BY sort_order ASC, name ASC
	`).Scan(ctx, &names)
	if err != nil {
		return nil, err
	}
	return names, nil
}
