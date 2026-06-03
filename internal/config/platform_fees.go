package config

// PlatformFeesConfig ส่วนแบ่งแพลตฟอร์มและผู้ขายเมื่อปิดประมูล
type PlatformFeesConfig struct {
	PlatformFeeNormalPct int64
	PlatformFeeEarlyPct  int64
	SellerKeepNormalPct  int64
	SellerKeepEarlyPct   int64
}
