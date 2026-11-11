// Package market reads stocks and price bars.
package market

import "github.com/t-011/mini_trading_platform/internal/database"

type Service struct {
	db *database.Queries
}

func New(db *database.Queries) *Service {
	return &Service{db: db}
}
