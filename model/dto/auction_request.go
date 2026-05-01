package dto

// AuctionWSClientMessage is a JSON message from the WebSocket client.
type AuctionWSClientMessage struct {
	Type   string `json:"type"`
	Amount int64  `json:"amount"`
}
