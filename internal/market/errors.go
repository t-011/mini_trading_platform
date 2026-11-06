package market

import "errors"

var (
	ErrStockNotFound = errors.New("stock not found")
	ErrBadInterval   = errors.New("interval must be one of: day, week, month")
)
