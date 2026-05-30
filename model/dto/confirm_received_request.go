package dto

// ConfirmReceivedRequest is the body for POST /auctions/:id/confirm-received.
type ConfirmReceivedRequest struct {
	Rating float64 `json:"rating"`
}
