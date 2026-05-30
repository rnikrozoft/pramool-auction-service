package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrNotWholeBaht is returned when a value has fractional baht (decimals).
	ErrNotWholeBaht = errors.New("amount must be whole baht without decimals")
	// ErrInvalidBaht is returned for empty, negative, or unparsable money amounts.
	ErrInvalidBaht = errors.New("invalid baht amount")
)

// ValidatePositiveBaht ensures amount is a positive whole baht (int64).
func ValidatePositiveBaht(amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("%w: must be positive", ErrInvalidBaht)
	}
	return nil
}

// ValidateNonNegativeBaht ensures amount is zero or a positive whole baht.
func ValidateNonNegativeBaht(amount int64) error {
	if amount < 0 {
		return fmt.Errorf("%w: cannot be negative", ErrInvalidBaht)
	}
	return nil
}

// ParseWholeBahtString parses form/query values that must not contain decimals.
func ParseWholeBahtString(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidBaht
	}
	if strings.ContainsAny(s, ".,") || strings.ContainsAny(s, "eE") {
		return 0, ErrNotWholeBaht
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidBaht, err)
	}
	return n, nil
}

// UnmarshalJSONInt64Baht decodes a JSON number only if it has no fractional part.
func UnmarshalJSONInt64Baht(data []byte) (int64, error) {
	data = bytesTrimSpace(data)
	if len(data) == 0 {
		return 0, ErrInvalidBaht
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err != nil {
		return 0, err
	}
	s := strings.TrimSpace(num.String())
	if strings.ContainsAny(s, ".") || strings.ContainsAny(s, "eE") {
		return 0, ErrNotWholeBaht
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidBaht, err)
	}
	return n, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
