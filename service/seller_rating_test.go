package service

import "testing"

func TestSellerPointsFromRating(t *testing.T) {
	cases := []struct {
		in      float64
		wantR   float64
		wantPts int
		wantErr bool
	}{
		{0.5, 0.5, 1, false},
		{1, 1, 2, false},
		{1.5, 1.5, 3, false},
		{4.5, 4.5, 9, false},
		{5, 5, 10, false},
		{0, 0, 0, true},
		{5.5, 0, 0, true},
		{1.3, 0, 0, true},
	}
	for _, tc := range cases {
		r, pts, err := SellerPointsFromRating(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("rating %.1f: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("rating %.1f: %v", tc.in, err)
		}
		if r != tc.wantR || pts != tc.wantPts {
			t.Fatalf("rating %.1f: got r=%.1f pts=%d", tc.in, r, pts)
		}
	}
}
