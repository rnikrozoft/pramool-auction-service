package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rnikrozoft/pramool-auction-service/model"
)

type Middleware struct {
	JWTSecret string
}

// JWTMiddleware matches pramool-core: CustomClaims user_id, then JWT sub fallback.
func (m Middleware) JWTMiddleware(c *fiber.Ctx) error {
	cookie := c.Cookies("access_token")
	if cookie == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "missing token"})
	}

	token, err := jwt.ParseWithClaims(cookie, &model.CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(m.JWTSecret), nil
	})
	var userID string
	if err == nil && token.Valid {
		if claims, ok := token.Claims.(*model.CustomClaims); ok {
			userID = strings.TrimSpace(claims.UserID)
			if userID == "" {
				userID = strings.TrimSpace(claims.RegisteredClaims.Subject)
			}
		}
	}
	if userID == "" {
		token2, err2 := jwt.Parse(cookie, func(token *jwt.Token) (interface{}, error) {
			return []byte(m.JWTSecret), nil
		})
		if err2 != nil || !token2.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "invalid token"})
		}
		if mc, ok := token2.Claims.(jwt.MapClaims); ok {
			if v, ok := mc["sub"].(string); ok {
				userID = strings.TrimSpace(v)
			}
		}
	}
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "invalid token"})
	}
	c.Locals("user_id", userID)
	return c.Next()
}
