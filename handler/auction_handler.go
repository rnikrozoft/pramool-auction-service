package handler

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/internal/money"
	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

type AuctionHandler struct {
	svc service.AuctionService
}

func NewAuctionHandler(svc service.AuctionService) *AuctionHandler {
	return &AuctionHandler{svc: svc}
}

func (h *AuctionHandler) ListingFees(c *fiber.Ctx) error {
	return c.JSON(h.svc.ListingFees(c.Context()))
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
	if n, ok := optionalWholeBahtQuery(c, "min_price"); ok && n != nil {
		f.MinPrice = n
	}
	if n, ok := optionalWholeBahtQuery(c, "max_price"); ok && n != nil {
		f.MaxPrice = n
	}
	if n, ok := optionalWholeBahtQuery(c, "min_start_price"); ok && n != nil {
		f.MinStartPrice = n
	}
	if n, ok := optionalWholeBahtQuery(c, "max_start_price"); ok && n != nil {
		f.MaxStartPrice = n
	}
	if n, ok := optionalWholeBahtQuery(c, "min_bid_step"); ok && n != nil {
		f.MinBidStep = n
	}
	if n, ok := optionalWholeBahtQuery(c, "max_bid_step"); ok && n != nil {
		f.MaxBidStep = n
	}
	if r, ok := optionalSellerRatingQuery(c, "min_seller_rating"); ok && r != nil {
		f.MinSellerRating = r
	}
	f.EndedScope = parseEndedScopeQuery(c)

	result, err := h.svc.ListPublicAuctions(c.Context(), f)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) ListAuctionBidders(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	result, err := h.svc.ListAuctionBidders(c.Context(), auctionID, limit)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "missing") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
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
	limit := 10
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	scope := strings.TrimSpace(c.Query("scope"))
	if scope == "" {
		scope = "all"
	}
	q := strings.TrimSpace(c.Query("q"))
	sort := strings.TrimSpace(c.Query("sort"))
	order := strings.TrimSpace(c.Query("order"))
	if order != "asc" {
		order = "desc"
	}
	result, err := h.svc.MyActiveBids(c.Context(), userID, scope, q, sort, order, limit, offset)
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
	limit := 10
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	scope := strings.TrimSpace(c.Query("scope"))
	if scope == "" {
		scope = "all"
	}
	q := strings.TrimSpace(c.Query("q"))
	sort := strings.TrimSpace(c.Query("sort"))
	order := strings.TrimSpace(c.Query("order"))
	if order != "asc" {
		order = "desc"
	}
	result, err := h.svc.MyBidHistory(c.Context(), userID, scope, q, sort, order, limit, offset)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
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
	var body dto.ConfirmReceivedRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}
	if err := h.svc.ConfirmBuyerReceived(c.Context(), auctionID, userID, body.Rating, body.Comment); err != nil {
		if errors.Is(err, service.ErrNotAuctionWinner) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "เฉพาะผู้ชนะประมูลเท่านั้นที่ยืนยันรับของได้"})
		}
		if errors.Is(err, service.ErrSellerMustShipFirst) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "รอผู้ขายบันทึกจัดส่งก่อน"})
		}
		if errors.Is(err, service.ErrShipmentNotDelivered) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "กรุณากดติดตามพัสดุและรอสถานะส่งถึงแล้วก่อนยืนยันรับของ"})
		}
		if errors.Is(err, service.ErrInvalidSellerRating) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "กรุณาให้คะแนนผู้ขาย 0.5–5 ดาว (กดได้ครึ่งดาว)"})
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

func (h *AuctionHandler) CancelBid(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	bidderID, _ := c.Locals("user_id").(string)
	if auctionID == "" || strings.TrimSpace(bidderID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request"})
	}
	result, err := h.svc.CancelBid(c.Context(), auctionID, bidderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBidCancelNotAllowed):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "รายการนี้ไม่เปิดให้ยกเลิกการบิด"})
		case errors.Is(err, service.ErrNoBidToCancel):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "ไม่มีการบิดที่ยกเลิกได้"})
		case errors.Is(err, service.ErrCannotBidOwn):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "ไม่สามารถยกเลิกการบิดรายการของตัวเองได้"})
		case errors.Is(err, service.ErrAuctionClosed):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "ประมูลปิดแล้ว"})
		case errors.Is(err, service.ErrSellerClosingAuction):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "ผู้ขายกำลังปิดประมูลชั่วคราว ไม่สามารถยกเลิกการบิดได้"})
		case errors.Is(err, service.ErrBidBanned):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "บัญชีถูกจำกัดการบิด"})
		default:
			if strings.Contains(err.Error(), "not found") {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "ไม่พบรายการประมูล"})
			}
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) ListProductCategories(c *fiber.Ctx) error {
	items, err := h.svc.ListProductCategories(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "ไม่สามารถโหลดหมวดหมู่ได้"})
	}
	return c.JSON(fiber.Map{"items": items})
}

func (h *AuctionHandler) ReportAuction(c *fiber.Ctx) error {
	auctionID := strings.TrimSpace(c.Params("id"))
	userID, _ := c.Locals("user_id").(string)
	if auctionID == "" || strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request"})
	}
	var body dto.ReportAuctionRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}
	resp, err := h.svc.ReportAuction(c.Context(), auctionID, userID, body.Reason)
	if err != nil {
		if errors.Is(err, service.ErrCannotReportOwn) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "ไม่สามารถร้องเรียนรายการของตัวเองได้"})
		}
		if errors.Is(err, repository.ErrReportDuplicatePending) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "คุณมีเรื่องร้องเรียนรอตรวจสอบสำหรับรายการนี้อยู่แล้ว"})
		}
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "ไม่พบรายการประมูล"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// optionalSellerRatingQuery parses min_seller_rating (0.5–5.0 half-star steps); invalid values are ignored.
func optionalSellerRatingQuery(c *fiber.Ctx, key string) (*float64, bool) {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return nil, true
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0.5 || f > 5.0 {
		return nil, false
	}
	steps := int(math.Round(f * 2))
	normalized := float64(steps) / 2.0
	if math.Abs(f-normalized) > 0.01 {
		return nil, false
	}
	return &normalized, true
}

func parseEndedScopeQuery(c *fiber.Ctx) string {
	switch strings.ToLower(strings.TrimSpace(c.Query("ended"))) {
	case "closed":
		return "closed"
	case "any":
		return "any"
	default:
		return "open"
	}
}

// optionalWholeBahtQuery parses a whole-baht filter query param; invalid decimals are ignored (nil, true).
func optionalWholeBahtQuery(c *fiber.Ctx, key string) (*int64, bool) {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return nil, true
	}
	n, err := money.ParseWholeBahtString(v)
	if err != nil {
		return nil, false
	}
	return &n, true
}
