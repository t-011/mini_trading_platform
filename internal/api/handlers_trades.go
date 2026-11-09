package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/trading"
)

func (cfg *Config) createTrade(w http.ResponseWriter, r *http.Request) {

	type params struct {
		Stock_id uuid.UUID `json:"stock_id"`
		Side     string    `json:"side"`
		Quantity string    `json:"quantity"`
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

	result, err := cfg.Trading.PlaceTrade(r.Context(), trading.PlaceTradeInput{
		UserID:   userID,
		StockID:  reqData.Stock_id,
		Side:     reqData.Side,
		Quantity: reqData.Quantity,
	})
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	// Embed the raw trade (PascalCase, same shape as GET /v1/trades) and add
	// the computed fill cost, which isn't stored on the trade row.
	respondWithJSON(w, http.StatusCreated, struct {
		database.Trade
		Cost int64
	}{Trade: result.Trade, Cost: result.Cost})
}
