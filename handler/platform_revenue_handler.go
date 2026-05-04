package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/uptrace/bun"
)

type PlatformRevenueHandler struct {
	db          *bun.DB
	internalKey string
}

func NewPlatformRevenueHandler(db *bun.DB, internalKey string) *PlatformRevenueHandler {
	return &PlatformRevenueHandler{db: db, internalKey: strings.TrimSpace(internalKey)}
}

// Summary GET /internal/platform-fee-summary — X-Internal-Key must match AUCTION_INTERNAL_KEY.
func (h *PlatformRevenueHandler) Summary(c *fiber.Ctx) error {
	if h.internalKey == "" || strings.TrimSpace(c.Get("X-Internal-Key")) != h.internalKey {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized internal call"})
	}
	out, err := repository.SummarizePlatformFeesAfterSellerPayout(c.Context(), h.db)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(out)
}
