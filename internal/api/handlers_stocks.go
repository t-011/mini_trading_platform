package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

const (
	DEFAULT_LIMIT = 50
	MIN_LIMIT     = 10
	MAX_LIMIT     = 100

	DEFAULT_OFFSET = 0
)

func (cfg *Config) getStocks(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-type", "application/json")

	search := r.URL.Query().Get("search")
	limit := clamp(parseIntQueries(r, "limit", DEFAULT_LIMIT), MIN_LIMIT, MAX_LIMIT)
	offset := parseIntQueries(r, "offset", DEFAULT_OFFSET)
	if offset < 0 {
		offset = 0
	}

	stocks, err := cfg.Market.ListStocks(r.Context(), search, int32(limit), int32(offset))
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Stocks []database.Stock `json:"stocks"`
	}

	respondWithJSON(w, http.StatusOK, respBody{Stocks: stocks})
}

func (cfg *Config) getStock(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-type", "application/json")

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	stock, err := cfg.Market.GetStock(r.Context(), id)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Stock database.Stock `json:"stock"`
	}

	respondWithJSON(w, http.StatusOK, respBody{Stock: stock})
}

func parseIntQueries(r *http.Request, key string, default_int int) int {
	num, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		num = default_int
	}
	return num
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
