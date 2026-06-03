package config

import (
	"fmt"
	"strconv"
	"strings"
)

// PlatformPolicy นโยบายจาก platform_settings (listing + fees + fulfillment)
type PlatformPolicy struct {
	Listing      ListingFeesConfig
	PlatformFees PlatformFeesConfig
	Fulfillment  FulfillmentConfig
}

func ParsePlatformPolicy(settings map[string]map[string]any) (PlatformPolicy, error) {
	listingM, err := requireSettingMap(settings, "listing")
	if err != nil {
		return PlatformPolicy{}, err
	}
	feesM, err := requireSettingMap(settings, "fees")
	if err != nil {
		return PlatformPolicy{}, err
	}
	fulM, err := requireSettingMap(settings, "fulfillment")
	if err != nil {
		return PlatformPolicy{}, err
	}
	listing, err := ParseListingFees(listingM)
	if err != nil {
		return PlatformPolicy{}, err
	}
	fees, err := ParsePlatformFees(feesM)
	if err != nil {
		return PlatformPolicy{}, err
	}
	fulfillment, err := ParseFulfillment(fulM)
	if err != nil {
		return PlatformPolicy{}, err
	}
	return PlatformPolicy{Listing: listing, PlatformFees: fees, Fulfillment: fulfillment}, nil
}

func ParseListingFees(m map[string]any) (ListingFeesConfig, error) {
	minStart, err := requireInt64Field(m, "min_start_price_thb", 1)
	if err != nil {
		return ListingFeesConfig{}, err
	}
	bidCancel, err := requireInt64Field(m, "bid_cancel_option_fee_thb", 0)
	if err != nil {
		return ListingFeesConfig{}, err
	}
	freeDays, err := requireInt64Field(m, "free_listing_duration_days", 1)
	if err != nil {
		return ListingFeesConfig{}, err
	}
	extraDayPct, err := requireInt64Field(m, "extra_listing_day_fee_pct", 0)
	if err != nil {
		return ListingFeesConfig{}, err
	}
	autoRenewOptionFee, err := requireInt64Field(m, "auto_renew_option_fee_thb", 0)
	if err != nil {
		return ListingFeesConfig{}, err
	}
	return ListingFeesConfig{
		MinStartPriceTHB:        minStart,
		BidCancelOptionFeeTHB:   bidCancel,
		FreeListingDurationDays: int(freeDays),
		ExtraListingDayFeePct:   extraDayPct,
		AutoRenewOptionFeeTHB:   autoRenewOptionFee,
	}, nil
}

func ParsePlatformFees(m map[string]any) (PlatformFeesConfig, error) {
	keepNormal, err := requireInt64Field(m, "seller_keep_normal_pct", 1)
	if err != nil {
		return PlatformFeesConfig{}, err
	}
	keepEarly, err := requireInt64Field(m, "seller_keep_early_pct", 1)
	if err != nil {
		return PlatformFeesConfig{}, err
	}
	if keepNormal > 100 || keepEarly > 100 {
		return PlatformFeesConfig{}, fmt.Errorf("platform_settings.fees seller_keep_* must be <= 100")
	}
	return PlatformFeesConfig{
		SellerKeepNormalPct:  keepNormal,
		PlatformFeeNormalPct: 100 - keepNormal,
		SellerKeepEarlyPct:   keepEarly,
		PlatformFeeEarlyPct:  100 - keepEarly,
	}, nil
}

func ParseFulfillment(m map[string]any) (FulfillmentConfig, error) {
	shipDays, err := requireInt64Field(m, "seller_ship_deadline_days", 1)
	if err != nil {
		return FulfillmentConfig{}, err
	}
	autoConfirm, err := requireInt64Field(m, "escrow_auto_confirm_days", 1)
	if err != nil {
		return FulfillmentConfig{}, err
	}
	return FulfillmentConfig{
		SellerShipDeadlineDays: int(shipDays),
		EscrowAutoConfirmDays:  int(autoConfirm),
	}, nil
}

func requireSettingMap(settings map[string]map[string]any, key string) (map[string]any, error) {
	if settings == nil {
		return nil, fmt.Errorf("platform_settings.%s missing (no settings loaded)", key)
	}
	m, ok := settings[key]
	if !ok || m == nil {
		return nil, fmt.Errorf("platform_settings.%s missing", key)
	}
	return m, nil
}

func requireInt64Field(m map[string]any, key string, min int64) (int64, error) {
	if m == nil {
		return 0, fmt.Errorf("platform_settings field %q missing", key)
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("platform_settings field %q missing", key)
	}
	v := jsonInt64(m, key)
	if v < min {
		return 0, fmt.Errorf("platform_settings field %q invalid: %d (min %d)", key, v, min)
	}
	return v, nil
}

func jsonInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}
