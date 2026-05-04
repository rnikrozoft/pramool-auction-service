package handler

import (
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

func (h *AuctionHandler) CreateSellerAuction(c *fiber.Ctx) error {
	sellerID, ok := c.Locals("user_id").(string)
	if !ok {
		return responseInternalError(c, errors.New("cannot claims user information"))
	}

	startPrice, err := strconv.ParseInt(c.FormValue("start_price"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid start_price"})
	}
	bidStep, err := strconv.ParseInt(c.FormValue("bid_step"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid bid_step"})
	}
	var buyNow int64
	if v := strings.TrimSpace(c.FormValue("buy_now_price")); v != "" {
		buyNow, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid buy_now_price"})
		}
	}

	req := dto.CreateAuctionRequest{
		Title:           c.FormValue("title"),
		Category:        c.FormValue("category"),
		Condition:       c.FormValue("condition"),
		Description:     c.FormValue("description"),
		StartPrice:      startPrice,
		BidStep:         bidStep,
		EndAt:           c.FormValue("end_at"),
		AllowEarlyClose: strings.EqualFold(strings.TrimSpace(c.FormValue("allow_early_close")), "true"),
		BuyNowPrice:     buyNow,
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid multipart form"})
	}
	files := form.File["images"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "at least one image is required"})
	}
	if len(files) > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "max 5 images"})
	}

	imagePaths := make([]string, 0, len(files))
	for i, file := range files {
		if err := validateSellerImageFile(file); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}

		root := strings.TrimSpace(os.Getenv("PRAMOOL_UPLOAD_ROOT"))
		if root == "" {
			root = "."
		}
		fileDir := filepath.Join(root, "uploads", "auctions", sellerID)
		if err := os.MkdirAll(fileDir, 0o755); err != nil {
			return responseInternalError(c, err)
		}

		fileName := fmt.Sprintf("%d-%d%s", time.Now().UnixNano(), i, sanitizeSellerImageExt(file.Filename))
		savePath := filepath.Join(fileDir, fileName)
		if err := c.SaveFile(file, savePath); err != nil {
			return responseInternalError(c, err)
		}
		publicPath := "/uploads/auctions/" + sellerID + "/" + fileName
		imagePaths = append(imagePaths, publicPath)
	}

	result, err := h.svc.CreateSellerAuction(c.Context(), sellerID, req, imagePaths)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "invalid category") ||
			strings.Contains(msg, "maximum ") ||
			strings.Contains(msg, "at least one category") ||
			strings.Contains(msg, "too long") ||
			strings.Contains(msg, "title is required") ||
			strings.Contains(msg, "invalid price settings") ||
			strings.Contains(msg, "invalid buy_now_price") ||
			strings.Contains(msg, "buy_now_price must") ||
			strings.Contains(msg, "invalid end_at") ||
			strings.Contains(msg, "end_at must be") ||
			strings.Contains(msg, "at least one image") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": msg})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *AuctionHandler) ListSellerAuctions(c *fiber.Ctx) error {
	sellerID, ok := c.Locals("user_id").(string)
	if !ok {
		return responseInternalError(c, errors.New("cannot claims user information"))
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

	result, err := h.svc.ListSellerAuctions(c.Context(), sellerID, scope, limit, offset)
	if err != nil {
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (h *AuctionHandler) ReopenSellerAuction(c *fiber.Ctx) error {
	sellerID, ok := c.Locals("user_id").(string)
	if !ok {
		return responseInternalError(c, errors.New("cannot claims user information"))
	}
	auctionID := strings.TrimSpace(c.Params("id"))
	if auctionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "missing auction id"})
	}
	var body struct {
		EndAt string `json:"end_at"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.EndAt) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "end_at is required (RFC3339)"})
	}
	err := h.svc.ReopenSellerAuctionNoBids(c.Context(), sellerID, auctionID, strings.TrimSpace(body.EndAt))
	if err != nil {
		if errors.Is(err, service.ErrAuctionReopenNotAllowed) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		if strings.Contains(strings.ToLower(err.Error()), "auction not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": err.Error()})
		}
		if strings.Contains(strings.ToLower(err.Error()), "invalid end_at") || strings.Contains(strings.ToLower(err.Error()), "end_at must be in the future") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		if strings.Contains(strings.ToLower(err.Error()), "insufficient credit") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		return responseInternalError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"auction_id": auctionID})
}

func (h *AuctionHandler) DeleteSellerAuction(c *fiber.Ctx) error {
	sellerID, ok := c.Locals("user_id").(string)
	if !ok {
		return responseInternalError(c, errors.New("cannot claims user information"))
	}
	auctionID := strings.TrimSpace(c.Params("id"))
	if auctionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "missing auction id"})
	}
	err := h.svc.DeleteSellerAuctionClosedNoBids(c.Context(), sellerID, auctionID)
	if err != nil {
		if errors.Is(err, service.ErrAuctionDeleteNotAllowed) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		if strings.Contains(strings.ToLower(err.Error()), "auction not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": err.Error()})
		}
		return responseInternalError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func validateSellerImageFile(file *multipart.FileHeader) error {
	maxBytes := int64(5 * 1024 * 1024)
	if file.Size > maxBytes {
		return fmt.Errorf("image %s exceeds 5MB", file.Filename)
	}

	contentType := strings.ToLower(file.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "image/jpeg") ||
		strings.HasPrefix(contentType, "image/jpg") ||
		strings.HasPrefix(contentType, "image/png") ||
		strings.HasPrefix(contentType, "image/webp") {
		return nil
	}
	return fmt.Errorf("unsupported image type for %s", file.Filename)
}

func sanitizeSellerImageExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	default:
		return ".jpg"
	}
}
