package model

import "github.com/golang-jwt/jwt/v5"

// CustomClaims mirrors pramool-core JWT shape for access_token cookies.
type CustomClaims struct {
	UserID   string
	LoggedIn bool
	jwt.RegisteredClaims
}
