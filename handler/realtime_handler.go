package handler

import (
	"context"
	"strings"
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/rnikrozoft/pramool-auction-service/model/dto"
	"github.com/rnikrozoft/pramool-auction-service/service"
)

type RealtimeHandler struct {
	hub *service.AuctionHub
	svc service.AuctionService
}

func NewRealtimeHandler(hub *service.AuctionHub, svc service.AuctionService) *RealtimeHandler {
	return &RealtimeHandler{hub: hub, svc: svc}
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
	defer h.hub.Leave(auctionID, client)

	ctx := context.Background()
	if detail, err := h.svc.GetAuctionDetail(ctx, auctionID); err == nil {
		_ = client.Send(dto.AuctionWSMessage{
			Type:       "snapshot",
			AuctionID:  auctionID,
			CurrentBid: detail.CurrentBid,
			TotalBids:  detail.TotalBids,
		})
	}

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
		out, err := h.svc.PlaceBid(ctx, auctionID, userID, req.Amount)
		if err != nil {
			_ = client.Send(dto.AuctionWSMessage{Type: "error", Message: err.Error()})
			continue
		}
		bidderID := out.BidderID
		if bidderID == "" {
			bidderID = userID
		}
		h.hub.Broadcast(auctionID, dto.AuctionWSMessage{
			Type:       "bid_update",
			AuctionID:  auctionID,
			BidderID:   bidderID,
			Amount:     req.Amount,
			CurrentBid: out.CurrentBid,
			TotalBids:  out.TotalBids,
		})
		_ = client.Send(dto.AuctionWSMessage{
			Type:            "bid_ack",
			AuctionID:       auctionID,
			RemainingCredit: out.RemainingCredit,
		})
	}
}
