package trading

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

// Holding is one position, derived from the user's trades. Monetary fields are
// cents; Qty stays an exact decimal string.
type Holding struct {
	StockID     uuid.UUID
	Symbol      string
	Name        string
	Qty         string
	AvgCost     int64 // weighted-avg buy price per share, cents
	LastClose   int64 // latest bar close (or last trade on our exchange), cents
	MarketValue int64 // Qty * LastClose
	CostBasis   int64 // Qty * AvgCost
	Unrealized  int64 // MarketValue - CostBasis
}

type Portfolio struct {
	Cash     int64
	Holdings []Holding
}

func (s *Service) GetCash(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.db.GetCash(ctx, userID)
}

func (s *Service) ListTrades(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]database.Trade, error) {
	return s.db.ListTradesByUser(ctx, database.ListTradesByUserParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
}

// GetPortfolio returns holdings plus cash.
// NOTE: two separate queries, not one snapshot. Use a read-only tx if that matters.
func (s *Service) GetPortfolio(ctx context.Context, userID uuid.UUID) (Portfolio, error) {
	rows, err := s.db.GetHoldings(ctx, userID)
	if err != nil {
		return Portfolio{}, err
	}
	cash, err := s.db.GetCash(ctx, userID)
	if err != nil {
		return Portfolio{}, err
	}

	holdings := make([]Holding, 0, len(rows))
	for _, r := range rows {
		holdings = append(holdings, deriveHolding(r))
	}
	return Portfolio{Cash: cash, Holdings: holdings}, nil
}

// NOTE: float math on the exact numeric strings from the DB. LastClose is the
// latest bar close, or the last trade price for our own exchange's stocks.
func deriveHolding(r database.GetHoldingsRow) Holding {
	qty, _ := strconv.ParseFloat(r.Qty, 64)
	avg, _ := strconv.ParseFloat(r.AvgCost, 64)
	mv := int64(math.Round(qty * float64(r.LastClose)))
	cb := int64(math.Round(qty * avg))
	return Holding{
		StockID:     r.StockID,
		Symbol:      r.Symbol,
		Name:        r.Name,
		Qty:         r.Qty,
		AvgCost:     int64(math.Round(avg)),
		LastClose:   r.LastClose,
		MarketValue: mv,
		CostBasis:   cb,
		Unrealized:  mv - cb,
	}
}

func (s *Service) ListNotes(ctx context.Context, userID, tradeID uuid.UUID) ([]database.TradeNote, error) {
	return s.db.ListNotesByTrade(ctx, database.ListNotesByTradeParams{
		TradeID: tradeID,
		UserID:  userID,
	})
}

// AddNote attaches a journal note to one of the caller's trades. The insert
// guards ownership in SQL: no row back means the trade isn't theirs.
func (s *Service) AddNote(ctx context.Context, userID, tradeID uuid.UUID, body string) (database.TradeNote, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return database.TradeNote{}, ErrBadNote
	}
	note, err := s.db.CreateNote(ctx, database.CreateNoteParams{
		TradeID: tradeID,
		Body:    body,
		UserID:  userID,
	})
	if err == sql.ErrNoRows {
		return database.TradeNote{}, ErrTradeNotFound
	}
	return note, err
}
