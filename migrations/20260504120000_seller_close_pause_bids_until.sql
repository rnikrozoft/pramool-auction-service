-- ช่วงที่ผู้ขายกดปิดก่อนเวลา: ไม่รับบิดจนกว่าจะ settle (หลัง ~3 วินาที)
ALTER TABLE auctions
  ADD COLUMN IF NOT EXISTS seller_close_pause_bids_until TIMESTAMPTZ NULL;

COMMENT ON COLUMN auctions.seller_close_pause_bids_until IS 'ถ้าไม่ NULL และ NOW() < ค่านี้ ระบบไม่รับบิดใหม่ (ผู้ขายกำลังปิดประมูล)';
