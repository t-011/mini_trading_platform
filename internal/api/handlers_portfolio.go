package api

import (
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/trading"
)

func (cfg *Config) getPortfolio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	p, err := cfg.Trading.GetPortfolio(r.Context(), userID)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, struct {
		Cash     int64             `json:"cash"`
		Holdings []trading.Holding `json:"holdings"`
	}{Cash: p.Cash, Holdings: p.Holdings})
}

func (cfg *Config) getCash(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cash, err := cfg.Trading.GetCash(r.Context(), userID)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, struct {
		Cash int64 `json:"cash"`
	}{Cash: cash})
}

func (cfg *Config) listTrades(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := clamp(parseIntQueries(r, "limit", DEFAULT_LIMIT), MIN_LIMIT, MAX_LIMIT)
	offset := parseIntQueries(r, "offset", DEFAULT_OFFSET)
	if offset < 0 {
		offset = 0
	}

	trades, err := cfg.Trading.ListTrades(r.Context(), userID, int32(limit), int32(offset))
	if err != nil {
		respondWithServiceError(w, err)
		return
	}
	if trades == nil {
		trades = []database.Trade{} // serialize as [] not null
	}

	respondWithJSON(w, http.StatusOK, struct {
		Trades []database.Trade `json:"trades"`
	}{Trades: trades})
}
