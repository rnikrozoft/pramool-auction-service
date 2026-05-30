package config

import (
	"os"
	"strconv"
	"strings"
)

// PlatformFeesConfig ส่วนแบ่งแพลตฟอร์มและผู้ขายเมื่อปิดประมูล (ควรสอดคล้องกับ wallet-service / หน้า terms)
type PlatformFeesConfig struct {
	// PlatformFeeNormalPct ค่าคอมมิชชันแพลตฟอร์มเมื่อปิดตามเวลา (% ของราคาผู้ชนะ)
	PlatformFeeNormalPct int64

	// PlatformFeeEarlyPct ค่าคอมมิชชันเมื่อผู้ขายปิดก่อนเวลา (%)
	PlatformFeeEarlyPct int64

	// SellerKeepNormalPct ส่วนที่เครดิตให้ผู้ขายหลังยืนยันรับของ — ปิดตามเวลา (%)
	SellerKeepNormalPct int64

	// SellerKeepEarlyPct ส่วนที่ให้ผู้ขายเมื่อปิดก่อนเวลา (%)
	SellerKeepEarlyPct int64
}

func DefaultPlatformFees() PlatformFeesConfig {
	return PlatformFeesConfig{
		PlatformFeeNormalPct: 25,
		PlatformFeeEarlyPct:  30,
		SellerKeepNormalPct:  75,
		SellerKeepEarlyPct:   70,
	}
}

// LoadPlatformFeesFromEnv ใช้ชื่อ env เดียวกับ wallet-service เพื่อตั้งค่าที่เดียว
func LoadPlatformFeesFromEnv() PlatformFeesConfig {
	cfg := DefaultPlatformFees()
	cfg.PlatformFeeNormalPct = envInt64("AUCTION_PLATFORM_FEE_NORMAL_PCT", cfg.PlatformFeeNormalPct)
	cfg.PlatformFeeEarlyPct = envInt64("AUCTION_PLATFORM_FEE_EARLY_PCT", cfg.PlatformFeeEarlyPct)
	cfg.SellerKeepNormalPct = envInt64("AUCTION_SELLER_KEEP_NORMAL_PCT", cfg.SellerKeepNormalPct)
	cfg.SellerKeepEarlyPct = envInt64("AUCTION_SELLER_KEEP_EARLY_PCT", cfg.SellerKeepEarlyPct)
	return cfg
}

func envInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
