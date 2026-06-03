package money

// ListingDepositPct is the seller listing deposit as a percent of start price.
const ListingDepositPct int64 = 10

// ListingDepositBaht returns whole-baht listing deposit (10% of start price, truncated).
func ListingDepositBaht(startPrice int64) int64 {
	if startPrice <= 0 {
		return 0
	}
	return (startPrice * ListingDepositPct) / 100
}
