// Package trading handles orders, cash and portfolios.
package trading

import (
	"database/sql"
	"sync"

	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/exchange"
)

type Service struct {
	db        *database.Queries
	conn      *sql.DB                     // raw pool, for beginning transactions
	exchanges map[string]*exchange.Client // engine connection per symbol on our exchange
	applyMu   sync.Mutex                  // serializes writing engine answers, see applyAnswer
}

// New builds the service. exchanges may be empty: orders on our exchange
// then fail with ErrExchangeDown, everything else still works.
func New(db *database.Queries, conn *sql.DB, exchanges map[string]*exchange.Client) *Service {
	return &Service{db: db, conn: conn, exchanges: exchanges}
}
