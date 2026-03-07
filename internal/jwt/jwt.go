package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AuthUser struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

func Generate(user AuthUser, expireIn time.Duration, secretKey string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expireIn)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email: user.Email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

func Validate(tokenStr, secretKey string) (*AuthUser, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user id in token")
	}

	return &AuthUser{ID: id, Email: claims.Email}, nil
}
