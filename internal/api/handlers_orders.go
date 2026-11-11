package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/trading"
)

func (cfg *Config) createOrder(w http.ResponseWriter, r *http.Request) {
	type params struct {
		StockID  uuid.UUID
		Side     string
		Quantity string // whole shares
		Price    int64  // limit, cents
	}

	w.Header().Set("Content-type", "application/json")

	var reqData params
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := cfg.Trading.PlaceOrder(r.Context(), trading.PlaceOrderInput{
		UserID:   userID,
		StockID:  reqData.StockID,
		Side:     reqData.Side,
		Quantity: reqData.Quantity,
		Price:    reqData.Price,
	})
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Order  database.Order
		Trades []database.Trade
	}

	respondWithJSON(w, http.StatusCreated, respBody{Order: result.Order, Trades: result.Trades})
}

func (cfg *Config) listOrders(w http.ResponseWriter, r *http.Request) {
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

	orders, err := cfg.Trading.ListOrders(r.Context(), userID, int32(limit), int32(offset))
	if err != nil {
		respondWithServiceError(w, err)
		return
	}
	if orders == nil {
		orders = []database.Order{} // serialize as [] not null
	}

	type respBody struct {
		Orders []database.Order
	}

	respondWithJSON(w, http.StatusOK, respBody{Orders: orders})
}

func (cfg *Config) cancelOrder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := cfg.Trading.CancelOrder(r.Context(), userID, orderID)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Order database.Order
	}

	respondWithJSON(w, http.StatusOK, respBody{Order: order})
}
