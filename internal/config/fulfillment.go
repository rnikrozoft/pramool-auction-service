package config

// FulfillmentConfig กำหนดส่งของ / ยืนยันรับอัตโนมัติ
type FulfillmentConfig struct {
	SellerShipDeadlineDays int
	EscrowAutoConfirmDays  int
}
