package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/market"
	"github.com/t-011/mini_trading_platform/internal/trading"
)

// respondWithServiceError maps a domain sentinel error to an HTTP status.
// This is the single place HTTP status is decided for service failures;
// unknown errors are logged and collapsed to a generic 500.
func respondWithServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trading.ErrBadSide), errors.Is(err, trading.ErrBadQuantity),
		errors.Is(err, market.ErrBadInterval):
		respondWithError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, trading.ErrNoPrice), errors.Is(err, market.ErrStockNotFound):
		respondWithError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, trading.ErrInsufficientFunds), errors.Is(err, trading.ErrInsufficientShares):
		respondWithError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		log.Printf("service error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
	}
}
