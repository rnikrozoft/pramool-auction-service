package service

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"
)

const maxReviewCommentRunes = 500

var ErrReviewCommentTooLong = errors.New("review comment too long")

var ErrInvalidSellerRating = errors.New("invalid seller rating")

// normalizeReviewComment trims and caps buyer review text (optional).
func normalizeReviewComment(comment string) string {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return ""
	}
	if utf8.RuneCountInString(comment) <= maxReviewCommentRunes {
		return comment
	}
	runes := []rune(comment)
	return strings.TrimSpace(string(runes[:maxReviewCommentRunes]))
}

// SellerPointsFromRating validates half-star rating (0.5–5.0) and returns seller points (rating × 2).
// Example: 1.0 stars → 2 points, 4.5 stars → 9 points.
func SellerPointsFromRating(rating float64) (float64, int, error) {
	steps := int(math.Round(rating * 2))
	if steps < 1 || steps > 10 {
		return 0, 0, ErrInvalidSellerRating
	}
	normalized := float64(steps) / 2.0
	if math.Abs(rating-normalized) > 0.01 {
		return 0, 0, ErrInvalidSellerRating
	}
	return normalized, steps, nil
}
