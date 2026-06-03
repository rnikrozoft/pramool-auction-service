package service

import (
	"testing"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/internal/config"
)

func testListingFees() config.ListingFeesConfig {
	return config.ListingFeesConfig{
		MinStartPriceTHB:        100,
		BidCancelOptionFeeTHB:   1,
		FreeListingDurationDays: 2,
		ExtraListingDayFeePct:   1,
		AutoRenewOptionFeeTHB:   20,
	}
}

func TestSellerListingDurationFeeBaht(t *testing.T) {
	cfg := testListingFees()
	if got := sellerListingDurationFeeBaht(cfg, 900, 3); got != 27 {
		t.Fatalf("sellerListingDurationFeeBaht(900, 3) = %d, want 27", got)
	}
	if got := sellerListingDurationFeeBaht(cfg, 900, 0); got != 0 {
		t.Fatalf("sellerListingDurationFeeBaht(900, 0) = %d, want 0", got)
	}
}

func TestExtraListingDaysBeyondFree(t *testing.T) {
	cfg := testListingFees()
	created := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	end := created.Add(5 * 24 * time.Hour)
	if got := extraListingDaysBeyondFree(cfg, created, end); got != 3 {
		t.Fatalf("extraListingDaysBeyondFree 5d span = %d, want 3", got)
	}
	end2 := created.Add(2 * 24 * time.Hour)
	if got := extraListingDaysBeyondFree(cfg, created, end2); got != 0 {
		t.Fatalf("extraListingDaysBeyondFree 2d span = %d, want 0", got)
	}
}

func TestParsePlatformPolicy(t *testing.T) {
	_, err := config.ParsePlatformPolicy(map[string]map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty settings")
	}
}
