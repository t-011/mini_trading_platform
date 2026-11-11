package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

func (cfg *Config) getNotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tradeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	notes, err := cfg.Trading.ListNotes(r.Context(), userID, tradeID)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Notes []database.TradeNote
	}

	respondWithJSON(w, http.StatusOK, respBody{Notes: notes})
}

func (cfg *Config) addNote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tradeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "{id} not uuid")
		return
	}

	type params struct {
		Text string
	}

	var reqData params
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	note, err := cfg.Trading.AddNote(r.Context(), userID, tradeID, reqData.Text)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Note database.TradeNote
	}

	respondWithJSON(w, http.StatusCreated, respBody{Note: note})
}
