package money

import "testing"

func TestSplitEscrowBySellerKeepPct(t *testing.T) {
	tests := []struct {
		name       string
		winner     int64
		keepPct    int64
		wantSeller int64
		wantPlat   int64
	}{
		{"exact 75/25", 1000, 75, 750, 250},
		{"remainder to platform", 1001, 75, 750, 251},
		{"early 70/30", 1001, 70, 700, 301},
		{"all platform", 500, 0, 0, 500},
		{"all seller", 500, 100, 500, 0},
		{"zero winner", 0, 75, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seller, plat := SplitEscrowBySellerKeepPct(tc.winner, tc.keepPct)
			if seller != tc.wantSeller || plat != tc.wantPlat {
				t.Fatalf("SplitEscrowBySellerKeepPct(%d, %d) = (%d, %d), want (%d, %d)",
					tc.winner, tc.keepPct, seller, plat, tc.wantSeller, tc.wantPlat)
			}
			if tc.winner > 0 && seller+plat != tc.winner {
				t.Fatalf("split sum %d != winner %d", seller+plat, tc.winner)
			}
		})
	}
}
