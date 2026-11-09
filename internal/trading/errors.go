package trading

import "errors"

var (
	ErrBadQuantity        = errors.New("quantity must be a positive number")
	ErrBadSide            = errors.New("side must be 'buy' or 'sell'")
	ErrNoPrice            = errors.New("nonexistent stock or no price available")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInsufficientShares = errors.New("insufficient shares")
	ErrTradeNotFound      = errors.New("trade not found")
	ErrBadNote            = errors.New("note text must not be empty")
)
