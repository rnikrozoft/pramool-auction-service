package money

import "testing"

func TestValidatePositiveBaht(t *testing.T) {
	if err := ValidatePositiveBaht(100); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePositiveBaht(0); err == nil {
		t.Fatal("expected error for zero")
	}
}

func TestParseWholeBahtString(t *testing.T) {
	n, err := ParseWholeBahtString("1000")
	if err != nil || n != 1000 {
		t.Fatalf("got %d %v", n, err)
	}
	for _, bad := range []string{"", "10.5", "10,5", "1e3", "  "} {
		if _, err := ParseWholeBahtString(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestUnmarshalJSONInt64Baht(t *testing.T) {
	n, err := UnmarshalJSONInt64Baht([]byte(`100`))
	if err != nil || n != 100 {
		t.Fatalf("got %d %v", n, err)
	}
	for _, raw := range []string{`100.5`, `1e2`, `""`} {
		if _, err := UnmarshalJSONInt64Baht([]byte(raw)); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
}
