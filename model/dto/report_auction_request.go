package dto

type ReportAuctionRequest struct {
	Reason string `json:"reason"`
}

type ReportAuctionResponse struct {
	ReportID int64  `json:"report_id"`
	Message  string `json:"message"`
}
