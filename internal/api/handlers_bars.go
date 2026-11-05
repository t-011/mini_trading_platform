package api

import (
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

func (cfg *Config) getBars(w http.ResponseWriter, r *http.Request) {
	// Right now we're only handling days, weeks, months
	// If we add lower time intervals, fix: parseDateQuery, valid map.

	w.Header().Set("Content-type", "application/json")

	to := parseDateQuery(r, "to", time.Now())
	from := parseDateQuery(r, "from", to.AddDate(0, -1, 0)) // default: last month
	interval := r.URL.Query().Get("interval")

	valid := map[string]bool{"day": true, "week": true, "month": true}
	if !valid[interval] {
		respondWithError(w, 400, "bad interval")
		return
	}

	stock_id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		log.Printf("getBars: {id} conversion failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	bars, err := cfg.Db.GetBarsByInterval(r.Context(), database.GetBarsByIntervalParams{
		Interval:      interval,
		StockID:       stock_id,
		TimestampFrom: from,
		TimestampTo:   to,
	})
	if err != nil {
		log.Printf("getBars: query failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	type respBody struct {
		Bars []database.GetBarsByIntervalRow `json:"bars"`
	}

	payload := respBody{
		Bars: bars,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func parseDateQuery(r *http.Request, key string, def time.Time) time.Time {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", s) // Go's reference date
	if err != nil {
		return def
	}
	return t
}
