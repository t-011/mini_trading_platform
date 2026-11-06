package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
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

	type respBody struct {
		Id        string `json:"id"`
		StockID   string `json:"stock_id"`
		Type      string `json:"type"`
		Qty       string `json:"qty"`
		Price     int64  `json:"price"`
		Cost      int64  `json:"cost"`
		Timestamp string `json:"timestamp"`
	}

	payload := respBody{
		Id:        result.Trade.ID.String(),
		StockID:   result.Trade.StockID.String(),
		Type:      result.Trade.Type,
		Qty:       result.Trade.Qty,
		Price:     result.Trade.Price,
		Cost:      result.Cost,
		Timestamp: result.Trade.Timestamp.String(),
	}

	respondWithJSON(w, http.StatusCreated, payload)
}
