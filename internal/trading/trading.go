// Package trading holds the trade/account/portfolio domain logic: order
// placement, cash movement, and position guards. It depends only on the
// store (internal/database) and knows nothing about HTTP.
package trading

import (
	"database/sql"

	"github.com/t-011/mini_trading_platform/internal/database"
)

type Service struct {
	db   *database.Queries
	conn *sql.DB // raw pool, for beginning transactions
}

func New(db *database.Queries, conn *sql.DB) *Service {
	return &Service{db: db, conn: conn}
}
