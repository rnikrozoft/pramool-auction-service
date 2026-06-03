package service

import (
	"fmt"
	"time"

	"github.com/rnikrozoft/pramool-auction-service/internal/config"
)

const listingDurationDay = 24 * time.Hour

func maxListingDurationFromCfg(cfg config.ListingFeesConfig) time.Duration {
	freeDays := cfg.FreeListingDurationDays
	if freeDays < 1 {
		freeDays = 1
	}
	return time.Duration(freeDays) * listingDurationDay
}

func validateAuctionEndAt(endAt time.Time) error {
	if !endAt.After(time.Now()) {
		return fmt.Errorf("end_at must be in the future")
	}
	return nil
}

func extraListingDaysBeyondFree(cfg config.ListingFeesConfig, createdAt, endAt time.Time) int {
	freeDays := cfg.FreeListingDurationDays
	if freeDays < 1 {
		freeDays = 1
	}
	maxDuration := time.Duration(freeDays) * listingDurationDay
	if endAt.Before(createdAt) {
		return 0
	}
	span := endAt.Sub(createdAt)
	if span <= maxDuration {
		return 0
	}
	excess := span - maxDuration
	days := int((excess + listingDurationDay - 1) / listingDurationDay)
	if days < 1 {
		return 1
	}
	return days
}

func sellerListingDurationFeeBaht(cfg config.ListingFeesConfig, winningBid int64, extraDays int) int64 {
	if extraDays <= 0 || winningBid <= 0 || cfg.ExtraListingDayFeePct <= 0 {
		return 0
	}
	fee := (winningBid * int64(extraDays) * cfg.ExtraListingDayFeePct) / 100
	if fee < 0 {
		return 0
	}
	return fee
}
