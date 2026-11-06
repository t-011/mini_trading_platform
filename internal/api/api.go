package api

import (
	"database/sql"
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/trading"
)

type Config struct {
	Db        *database.Queries
	DbConn    *sql.DB // raw pool, for beginning transactions
	JwtSecret string

	Trading *trading.Service
}

func NewHandler(cfg *Config) http.Handler {
	mux := http.NewServeMux()

	// Auth
	mux.HandleFunc("POST /v1/users", cfg.createUserHandler)
	mux.Handle("PUT /v1/users", cfg.middlewareAuth(http.HandlerFunc(cfg.updateUserHandler)))
	mux.HandleFunc("POST /v1/login", cfg.loginHandler)
	mux.HandleFunc("POST /v1/refresh", cfg.refreshTokenHandler)
	mux.HandleFunc("POST /v1/revoke", cfg.revokeTokenHandler)

	// Stocks (public, read-only)
	mux.HandleFunc("GET /v1/stocks", cfg.getStocks)
	mux.HandleFunc("GET /v1/stocks/{id}", cfg.getStock)
	mux.HandleFunc("GET /v1/stocks/{id}/bars", cfg.getBars)

	// Trades (auth required)
	mux.Handle("POST /v1/trades", cfg.middlewareAuth(http.HandlerFunc(cfg.createTrade)))

	return mux
}
