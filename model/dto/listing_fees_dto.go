package dto

// ListingFeesResponse ค่าธรรมเนียมตอนสร้างประมูลและตัวเลือกเสริม (public)
type ListingFeesResponse struct {
	MinStartPriceTHB         int64 `json:"min_start_price_thb"`
	BidCancelOptionFeeTHB    int64 `json:"bid_cancel_option_fee_thb"`
	FreeListingDurationDays  int   `json:"free_listing_duration_days"`
	ExtraListingDayFeePct    int64 `json:"extra_listing_day_fee_pct"`
	AutoRenewOptionFeeTHB    int64 `json:"auto_renew_option_fee_thb"`
}
