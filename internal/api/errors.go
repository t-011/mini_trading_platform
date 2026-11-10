package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/market"
	"github.com/t-011/mini_trading_platform/internal/trading"
	"github.com/t-011/mini_trading_platform/internal/users"
)

// respondWithServiceError maps a domain sentinel error to an HTTP status.
// This is the single place HTTP status is decided for service failures;
// unknown errors are logged and collapsed to a generic 500.
func respondWithServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trading.ErrBadSide), errors.Is(err, trading.ErrBadQuantity),
		errors.Is(err, trading.ErrBadNote), errors.Is(err, trading.ErrBadPrice),
		errors.Is(err, market.ErrBadInterval), errors.Is(err, users.ErrMissingFields):
		respondWithError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, users.ErrInvalidCredentials), errors.Is(err, users.ErrInvalidToken):
		respondWithError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, users.ErrEmailTaken):
		respondWithError(w, http.StatusConflict, err.Error())
	case errors.Is(err, trading.ErrTradeNotFound),
		errors.Is(err, trading.ErrStockNotFound), errors.Is(err, trading.ErrOrderNotFound),
		errors.Is(err, market.ErrStockNotFound):
		respondWithError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, trading.ErrInsufficientFunds), errors.Is(err, trading.ErrInsufficientShares),
		errors.Is(err, trading.ErrNotTradable):
		respondWithError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, trading.ErrOrderNotOpen):
		respondWithError(w, http.StatusConflict, err.Error())
	case errors.Is(err, trading.ErrExchangeDown):
		respondWithError(w, http.StatusServiceUnavailable, err.Error())
	default:
		log.Printf("service error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "")
	}
}
