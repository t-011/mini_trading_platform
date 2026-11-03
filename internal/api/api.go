package api

import (
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/database"
)

type Config struct {
	Db             *database.Queries
	JwtSecret 	   string
}

func NewHandler(cfg *Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/users", cfg.createUserHandler)
	mux.Handle("PUT /api/users", cfg.middlewareAuth(http.HandlerFunc(cfg.updateUserHandler)))
	mux.HandleFunc("POST /api/login", cfg.loginHandler)
	mux.HandleFunc("POST /api/refresh", cfg.refreshTokenHandler)
	mux.HandleFunc("POST /api/revoke", cfg.revokeTokenHandler)

	return mux
}
