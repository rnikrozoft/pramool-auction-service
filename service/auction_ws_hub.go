package service

import (
	"sync"

	"github.com/rnikrozoft/pramool-auction-service/model/dto"
)

type AuctionWSClient interface {
	Send(message dto.AuctionWSMessage) error
}

type AuctionHub struct {
	mu    sync.RWMutex
	rooms map[string]map[AuctionWSClient]struct{}
}

func NewAuctionHub() *AuctionHub {
	return &AuctionHub{rooms: make(map[string]map[AuctionWSClient]struct{})}
}

func (h *AuctionHub) Join(auctionID string, client AuctionWSClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[auctionID]; !ok {
		h.rooms[auctionID] = make(map[AuctionWSClient]struct{})
	}
	h.rooms[auctionID][client] = struct{}{}
}

func (h *AuctionHub) Leave(auctionID string, client AuctionWSClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients, ok := h.rooms[auctionID]
	if !ok {
		return
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.rooms, auctionID)
	}
}

func (h *AuctionHub) Broadcast(auctionID string, message dto.AuctionWSMessage) {
	h.mu.RLock()
	clients := h.rooms[auctionID]
	h.mu.RUnlock()
	for client := range clients {
		_ = client.Send(message)
	}
}

// RoomSize returns connected WebSocket clients in an auction room (in-memory, single instance).
func (h *AuctionHub) RoomSize(auctionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[auctionID])
}

// NotifyRoomPresence broadcasts type=presence with current viewer_count to everyone in the room.
func (h *AuctionHub) NotifyRoomPresence(auctionID string) {
	h.Broadcast(auctionID, dto.AuctionWSMessage{
		Type:        "presence",
		AuctionID:   auctionID,
		ViewerCount: h.RoomSize(auctionID),
	})
}
