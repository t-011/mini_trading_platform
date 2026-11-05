package api

import (
	"database/sql"
	"log"
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

	query := r.URL.Query()

	search := query.Get("search")
	limit := clamp(parseIntQueries(r, "limit", DEFAULT_LIMIT), MIN_LIMIT, MAX_LIMIT)
	offset := parseIntQueries(r, "offset", DEFAULT_OFFSET)
	if offset < 0 {
		offset = 0
	}

	stocks, err := cfg.Db.GetStockBySymbolOrName(r.Context(), database.GetStockBySymbolOrNameParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		Search: sql.NullString{String: search, Valid: true},
	})
	if err != nil {
		log.Printf("getStocks: query failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	type respBody struct {
		Stocks []database.Stock `json:"stocks"`
	}

	payload := respBody{
		Stocks: stocks,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func (cfg *Config) getStock(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-type", "application/json")

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		log.Printf("getStock: {id} conversion failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	stock, err := cfg.Db.GetStockById(r.Context(), id)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "stock not found")
		return
	}

	if err != nil {
		log.Printf("getStock: query failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	type respBody struct {
		Stock database.Stock `json:"stock"`
	}

	payload := respBody{
		Stock: stock,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func parseIntQueries(r *http.Request, key string, default_int int) int {
	query := r.URL.Query()

	num, err := strconv.Atoi(query.Get(key))

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
