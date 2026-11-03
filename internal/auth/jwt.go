package auth

import (
	"net/http"
	"strings"
	"time"

	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "mini-trading-access",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:   userID.String(),
	})

	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			return []byte(tokenSecret), nil
		},
	)
	if err != nil || !token.Valid {
		return uuid.Nil, err
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, errors.New("missing sub claim")
	}

	return uuid.Parse(sub)
}

func GetBearerToken(headers http.Header) (string, error) {
	parts := strings.Fields(headers.Get("Authorization"))

	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid Authorization header")
	}

	return parts[1], nil
}
