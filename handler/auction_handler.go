package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

type AuctionHandler struct {
	svc service.AuctionService
}

func NewAuctionHandler(svc service.AuctionService) *AuctionHandler {
	return &AuctionHandler{svc: svc}
}

func (h *AuctionHandler) AuctionDetail(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	result, err := h.svc.GetAuctionDetail(c.Context(), auctionID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "not found"})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}
