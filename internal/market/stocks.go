package market

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

// ListStocks returns stocks matching an optional search term (symbol or name),
// paged. Empty search matches all.
func (s *Service) ListStocks(ctx context.Context, search string, limit, offset int32) ([]database.Stock, error) {
	return s.db.GetStockBySymbolOrName(ctx, database.GetStockBySymbolOrNameParams{
		Limit:  limit,
		Offset: offset,
		Search: sql.NullString{String: search, Valid: true},
	})
}

// GetStock returns a single stock by id, mapping "no row" to ErrStockNotFound.
func (s *Service) GetStock(ctx context.Context, id uuid.UUID) (database.Stock, error) {
	stock, err := s.db.GetStockById(ctx, id)
	if err == sql.ErrNoRows {
		return database.Stock{}, ErrStockNotFound
	}
	return stock, err
}
