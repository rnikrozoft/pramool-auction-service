package money

// SplitEscrowBySellerKeepPct แบ่งยอด escrow ผู้ชนะ (บาทเต็ม).
// ผู้ขายได้ส่วน = trunc(winner × sellerKeepPct / 100); เศษทั้งหมด (รวมส่วนที่หารไม่ลงตัว) = แพลตฟอร์ม.
func SplitEscrowBySellerKeepPct(winnerAmount, sellerKeepPct int64) (sellerShare, platformFee int64) {
	if winnerAmount <= 0 {
		return 0, 0
	}
	if sellerKeepPct <= 0 {
		return 0, winnerAmount
	}
	if sellerKeepPct >= 100 {
		return winnerAmount, 0
	}
	sellerShare = (winnerAmount * sellerKeepPct) / 100
	platformFee = winnerAmount - sellerShare
	return sellerShare, platformFee
}
