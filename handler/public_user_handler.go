package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (h *AuctionHandler) PublicUserProfile(c *fiber.Ctx) error {
	userID := strings.TrimSpace(c.Params("id"))
	activeLimit := 24
	if v := strings.TrimSpace(c.Query("active_limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			activeLimit = n
		}
	}
	reviewsLimit := 50
	if v := strings.TrimSpace(c.Query("reviews_limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			reviewsLimit = n
		}
	}

	result, err := h.svc.GetPublicUserProfile(c.Context(), userID, activeLimit, reviewsLimit)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "not found"})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) PublicUserClosedAuctions(c *fiber.Ctx) error {
	userID := strings.TrimSpace(c.Params("id"))
	limit := 24
	offset := 0
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	result, err := h.svc.ListPublicUserClosedAuctions(c.Context(), userID, limit, offset)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "not found"})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}
