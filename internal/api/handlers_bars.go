package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/market"
)

func (cfg *Config) getBars(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-type", "application/json")

	stockID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	query := r.URL.Query()
	bars, err := cfg.Market.GetBars(r.Context(), market.GetBarsInput{
		StockID:  stockID,
		From:     query.Get("from"),
		To:       query.Get("to"),
		Interval: query.Get("interval"),
	})
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Bars []database.GetBarsByIntervalRow `json:"bars"`
	}

	respondWithJSON(w, http.StatusOK, respBody{Bars: bars})
}
