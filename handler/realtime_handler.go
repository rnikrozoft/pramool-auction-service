package handler

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rnikrozoft/pramool-auction-service/internal/auctionlive"
	"github.com/rnikrozoft/pramool-auction-service/internal/money"
	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

type RealtimeHandler struct {
	hub      *service.AuctionHub
	svc      service.AuctionService
	wsFanout auctionlive.WSFanout
}

func NewRealtimeHandler(hub *service.AuctionHub, svc service.AuctionService, wsFanout auctionlive.WSFanout) *RealtimeHandler {
	if wsFanout == nil {
		wsFanout = auctionlive.LocalWSFanout()
	}
	return &RealtimeHandler{hub: hub, svc: svc, wsFanout: wsFanout}
}

func (h *RealtimeHandler) deliverLocal(auctionID string, message dto.AuctionWSMessage) {
	h.hub.Broadcast(auctionID, message)
}

// AuctionPresence returns in-memory WebSocket viewer counts per auction (GET /auctions/presence?ids=a,b,c).
func (h *RealtimeHandler) AuctionPresence(c *fiber.Ctx) error {
	raw := strings.TrimSpace(c.Query("ids"))
	if raw == "" {
		return c.JSON(dto.AuctionPresenceResponse{Counts: map[string]int{}})
	}
	parts := strings.Split(raw, ",")
	counts := make(map[string]int, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if len(counts) >= 100 {
			break
		}
		counts[id] = h.hub.RoomSize(id)
	}
	return c.JSON(dto.AuctionPresenceResponse{Counts: counts})
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) Send(message dto.AuctionWSMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(message)
}

func (h *RealtimeHandler) AuctionWS(conn *websocket.Conn) {
	auctionID := conn.Params("id")
	userID, _ := conn.Locals("user_id").(string)
	client := &wsClient{conn: conn}
	h.hub.Join(auctionID, client)
	defer func() {
		h.hub.Leave(auctionID, client)
		h.hub.NotifyRoomPresence(auctionID)
	}()

	ctx := context.Background()
	if detail, err := h.svc.GetAuctionDetail(ctx, auctionID); err == nil {
		_ = client.Send(dto.AuctionWSMessage{
			Type:               "snapshot",
			AuctionID:          auctionID,
			CurrentBid:         detail.CurrentBid,
			TotalBids:          detail.TotalBids,
			EndAt:              detail.EndAt,
			BiddingPausedUntil: detail.BiddingPausedUntil,
			ViewerCount:        h.hub.RoomSize(auctionID),
		})
	}
	h.hub.NotifyRoomPresence(auctionID)

	for {
		var req dto.AuctionWSClientMessage
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		if req.Type != "bid" {
			_ = client.Send(dto.AuctionWSMessage{Type: "error", Message: "unsupported message type"})
			continue
		}
		if strings.TrimSpace(userID) == "" {
			_ = client.Send(dto.AuctionWSMessage{Type: "error", Message: "missing user"})
			continue
		}
		if err := money.ValidatePositiveBaht(req.Amount); err != nil {
			_ = client.Send(dto.AuctionWSMessage{Type: "error", Message: "จำนวนเงินต้องเป็นบาทเต็ม (ไม่มีทศนิยม)"})
			continue
		}
		out, err := h.svc.PlaceBid(ctx, auctionID, userID, req.Amount)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, service.ErrBidBanned) {
				msg = "บัญชีถูกระงับการประมูล"
			}
			if errors.Is(err, service.ErrCreditDebt) {
				msg = "คุณมียอดค้างชำระ กรุณาเติมเครดิตให้ครบก่อนประมูล"
			}
			_ = client.Send(dto.AuctionWSMessage{Type: "error", Message: msg})
			continue
		}
		bidderID := out.BidderID
		if bidderID == "" {
			bidderID = userID
		}
		bidUpdate := dto.AuctionWSMessage{
			Type:       "bid_update",
			AuctionID:  auctionID,
			BidderID:   bidderID,
			Amount:     req.Amount,
			CurrentBid: out.CurrentBid,
			TotalBids:  out.TotalBids,
			EndAt:      out.EndAt,
		}
		_ = h.wsFanout.Publish(ctx, auctionID, bidUpdate, h.deliverLocal)
		_ = client.Send(dto.AuctionWSMessage{
			Type:            "bid_ack",
			AuctionID:       auctionID,
			RemainingCredit: out.RemainingCredit,
			AuctionClosed:   out.AuctionClosed,
			EndAt:           out.EndAt,
			CurrentBid:      out.CurrentBid,
			TotalBids:       out.TotalBids,
		})
	}
}
