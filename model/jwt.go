package model

import "github.com/golang-jwt/jwt/v5"

// CustomClaims mirrors pramool-core JWT shape for access_token cookies.
type CustomClaims struct {
	UserID   string `json:"user_id,omitempty"`
	LoggedIn bool   `json:"logged_in,omitempty"`
	TokenUse string `json:"token_use,omitempty"`
	jwt.RegisteredClaims
}
