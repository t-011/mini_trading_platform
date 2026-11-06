package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
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
		respondWithError(w, http.StatusBadRequest, "stock_id not valid uuid")
		return
	}

	valid_sides := map[string]bool{"buy": true, "sell": true}
	if !valid_sides[reqData.Side] {
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid side %q: must be 'buy' or 'sell'", reqData.Side))
		return
	}

	qty, err := strconv.ParseFloat(reqData.Quantity, 64)
	if err != nil || qty <= 0 {
		respondWithError(w, http.StatusBadRequest, "bad quantity")
		return
	}

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tx, err := cfg.DbConn.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("create trade: begin tx: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}
	defer tx.Rollback()

	qtx := cfg.Db.WithTx(tx)

	closeAmount, err := qtx.GetLatestClose(r.Context(), reqData.Stock_id)
	if err == sql.ErrNoRows {
		// we're collapsing the errors: (stock_id not found, no bars for stock exist)
		respondWithError(w, http.StatusNotFound, "nonexistent stock or no price available")
		return
	}
	if err != nil {
		log.Printf("trade: get close: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	raw := float64(closeAmount) * qty // cents, fractional

	var cost int64 // round in favor of the house: buy up, sell down
	if reqData.Side == "buy" {
		cost = int64(math.Ceil(raw))

		res, err := qtx.Debit(r.Context(), database.DebitParams{
			Cost:   cost,
			UserID: userID,
		})
		if err != nil {
			log.Printf("trade: debit: %v", err)
			respondWithError(w, http.StatusInternalServerError, "")
			return
		}
		if res == 0 {
			respondWithError(w, http.StatusUnprocessableEntity, "insufficient funds")
			return
		}

	} else {
		cost = int64(math.Floor(raw))

		positionStr, err := qtx.GetPosition(r.Context(), database.GetPositionParams{
			UserID: userID,
			StockID: reqData.Stock_id,
		})

		if err != nil {
			log.Printf("trade: get position: %v", err)
			respondWithError(w, http.StatusInternalServerError, "")
			return
		}

		position, err := strconv.ParseFloat(positionStr, 64)
		if err != nil {
			log.Printf("trade: parse position: %v", err)
			respondWithError(w, http.StatusInternalServerError, "")
			return
		}

		// ponytail: float share compare, exact-decimal if it ever misfires
		if position < qty {
			respondWithError(w, http.StatusUnprocessableEntity, "insufficient shares")
			return
		}

		err = qtx.Credit(r.Context(), database.CreditParams{
			Cost:   cost,
			UserID: userID,
		})

		if err != nil {
			log.Printf("trade: credit: %v", err)
			respondWithError(w, http.StatusInternalServerError, "")
			return
		}
	}

	trade, err := qtx.CreateTrade(r.Context(), database.CreateTradeParams{
		UserID:  userID,
		StockID: reqData.Stock_id,
		Type:    reqData.Side,
		Qty:     reqData.Quantity, // exact string, no float
		Price:   closeAmount,      // per-share fill price, cents
	})
	if err != nil {
		log.Printf("create trade: insert: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	if err = tx.Commit(); err != nil {
		log.Printf("create trade: commit: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
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
		Id:        trade.ID.String(),
		StockID:   trade.StockID.String(),
		Type:      trade.Type,
		Qty:       trade.Qty,
		Price:     trade.Price,
		Cost:      cost,
		Timestamp: trade.Timestamp.String(),
	}

	respondWithJSON(w, http.StatusCreated, payload)
}
