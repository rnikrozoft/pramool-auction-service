package config

// ListingFeesConfig ค่าธรรมเนียม/มัดจำตอนสร้างประมูลและเมื่อปิดขายสำเร็จ (เกินระยะ / ต่ออายุ)
type ListingFeesConfig struct {
	MinStartPriceTHB        int64
	BidCancelOptionFeeTHB   int64
	FreeListingDurationDays int
	ExtraListingDayFeePct   int64
	AutoRenewOptionFeeTHB   int64
}
