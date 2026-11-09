package api

import (
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/market"
	"github.com/t-011/mini_trading_platform/internal/trading"
	"github.com/t-011/mini_trading_platform/internal/users"
)

type Config struct {
	JwtSecret string // used by the auth middleware to validate request tokens

	Users   *users.Service
	Market  *market.Service
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
	mux.Handle("GET /v1/trades", cfg.middlewareAuth(http.HandlerFunc(cfg.listTrades)))
	mux.Handle("GET /v1/trades/{id}/notes", cfg.middlewareAuth(http.HandlerFunc(cfg.getNotes)))
	mux.Handle("POST /v1/trades/{id}/notes", cfg.middlewareAuth(http.HandlerFunc(cfg.addNote)))

	// Portfolio (auth required)
	mux.Handle("GET /v1/portfolio", cfg.middlewareAuth(http.HandlerFunc(cfg.getPortfolio)))
	mux.Handle("GET /v1/cash", cfg.middlewareAuth(http.HandlerFunc(cfg.getCash)))

	return mux
}
