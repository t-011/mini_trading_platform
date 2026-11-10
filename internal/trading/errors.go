package trading

import "errors"

var (
	ErrBadQuantity        = errors.New("quantity must be a positive whole number of shares")
	ErrBadSide            = errors.New("side must be 'buy' or 'sell'")
	ErrNoPrice            = errors.New("nonexistent stock or no price available")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInsufficientShares = errors.New("insufficient shares")
	ErrTradeNotFound      = errors.New("trade not found")
	ErrBadNote            = errors.New("note text must not be empty")
	ErrBadPrice           = errors.New("price must be a positive number of cents")
	ErrStockNotFound      = errors.New("stock not found")
	ErrNotTradable        = errors.New("stock is read-only; only stocks on our exchange can be traded")
	ErrExchangeDown       = errors.New("exchange unavailable")
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderNotOpen       = errors.New("order is not open")
)
