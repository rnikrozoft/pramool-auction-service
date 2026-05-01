package entity

type Bidder struct {
	UserID string `db:"user_id"`
	Credit int64  `db:"credit"`
}
