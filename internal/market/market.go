// Package market holds reference-data and market-data reads: the stock
// catalog and aggregated price bars. Read-only; depends only on the store.
package market

import "github.com/t-011/mini_trading_platform/internal/database"

type Service struct {
	db *database.Queries
}

func New(db *database.Queries) *Service {
	return &Service{db: db}
}
