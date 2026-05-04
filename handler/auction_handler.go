package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

type AuctionHandler struct {
	svc service.AuctionService
}

func NewAuctionHandler(svc service.AuctionService) *AuctionHandler {
	return &AuctionHandler{svc: svc}
}

func (h *AuctionHandler) ListAuctions(c *fiber.Ctx) error {
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 500000 {
			offset = n
		}
	}
	sort := strings.TrimSpace(c.Query("sort"))
	if sort == "" {
		sort = "newest"
	}

	f := repository.PublicAuctionFilter{
		Query:       strings.TrimSpace(c.Query("q")),
		Category:    strings.TrimSpace(c.Query("category")),
		Sort:        sort,
		Limit:       limit,
		Offset:      offset,
		EndFromDate: strings.TrimSpace(c.Query("end_from")),
		EndToDate:   strings.TrimSpace(c.Query("end_to")),
	}
	if v := strings.TrimSpace(c.Query("min_price")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MinPrice = &n
		}
	}
	if v := strings.TrimSpace(c.Query("max_price")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MaxPrice = &n
		}
	}
	if v := strings.TrimSpace(c.Query("min_start_price")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MinStartPrice = &n
		}
	}
	if v := strings.TrimSpace(c.Query("max_start_price")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MaxStartPrice = &n
		}
	}
	if v := strings.TrimSpace(c.Query("min_bid_step")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MinBidStep = &n
		}
	}
	if v := strings.TrimSpace(c.Query("max_bid_step")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			f.MaxBidStep = &n
		}
	}

	result, err := h.svc.ListPublicAuctions(c.Context(), f)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
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

func (h *AuctionHandler) MyActiveBids(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)
	if strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	result, err := h.svc.MyActiveBids(c.Context(), userID)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) MyBidHistory(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)
	if strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 500000 {
			offset = n
		}
	}
	result, err := h.svc.MyBidHistory(c.Context(), userID, limit, offset)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) MarkSellerShipped(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	userID, _ := c.Locals("user_id").(string)
	if strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	if auctionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid auction"})
	}
	if err := h.svc.MarkSellerShipped(c.Context(), auctionID, userID); err != nil {
		if errors.Is(err, service.ErrMarkShippedNotAllowed) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "ไม่สามารถบันทึกการจัดส่งได้"})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "บันทึกการจัดส่งแล้ว"})
}

func (h *AuctionHandler) ConfirmBuyerReceived(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	userID, _ := c.Locals("user_id").(string)
	if strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	if auctionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid auction"})
	}
	if err := h.svc.ConfirmBuyerReceived(c.Context(), auctionID, userID); err != nil {
		if errors.Is(err, service.ErrNotAuctionWinner) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "เฉพาะผู้ชนะประมูลเท่านั้นที่ยืนยันรับของได้"})
		}
		if errors.Is(err, service.ErrSellerMustShipFirst) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "รอผู้ขายบันทึกจัดส่งก่อน"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "ยืนยันรับของแล้ว ระบบโอนเครดิตให้ผู้ขาย"})
}

func (h *AuctionHandler) CloseEarly(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	sellerID, _ := c.Locals("user_id").(string)
	if auctionID == "" || strings.TrimSpace(sellerID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request"})
	}
	if err := h.svc.CloseAuctionEarly(c.Context(), auctionID, sellerID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "accepted"})
}
