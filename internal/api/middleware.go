package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/auth"
)

type userIDContextKey struct{}

func (cfg *Config) middlewareAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			respondWithError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		userID, err := auth.ValidateJWT(token, cfg.JwtSecret)
		if err != nil || userID == uuid.Nil {
			w.Header().Set("Content-Type", "application/json")
			respondWithError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	return userID, ok
}
