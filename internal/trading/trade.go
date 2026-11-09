package trading

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

type PlaceTradeInput struct {
	UserID   uuid.UUID
	StockID  uuid.UUID
	Side     string // "buy" or "sell"
	Quantity string // decimal string, stored exact
}

type TradeResult struct {
	Trade database.Trade
	Cost  int64 // cents moved for this fill
}

// PlaceTrade fills a market order at the latest bar close, moves cash, and
// records the trade — all in one transaction. It returns typed sentinel
// errors (ErrInsufficientFunds, etc.) for the transport layer to map.
func (s *Service) PlaceTrade(ctx context.Context, in PlaceTradeInput) (TradeResult, error) {
	if in.Side != "buy" && in.Side != "sell" {
		return TradeResult{}, ErrBadSide
	}

	qty, err := strconv.ParseFloat(in.Quantity, 64)
	if err != nil || qty <= 0 {
		return TradeResult{}, ErrBadQuantity
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return TradeResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.db.WithTx(tx)

	// Lock the account row up front so a user's concurrent trades serialize.
	// Without this, two sells could both pass the read-then-write position
	// check below and oversell the holding. Buys were already safe (atomic
	// conditional Debit), but locking here covers both paths uniformly.
	if err := qtx.LockAccount(ctx, in.UserID); err != nil {
		return TradeResult{}, fmt.Errorf("lock account: %w", err)
	}

	closeAmount, err := qtx.GetLatestClose(ctx, in.StockID)
	if err == sql.ErrNoRows {
		// collapses two cases: unknown stock, or stock with no bars yet
		return TradeResult{}, ErrNoPrice
	}
	if err != nil {
		return TradeResult{}, fmt.Errorf("get latest close: %w", err)
	}

	cost := houseCost(closeAmount, qty, in.Side)

	if in.Side == "buy" {
		res, err := qtx.Debit(ctx, database.DebitParams{Cost: cost, UserID: in.UserID})
		if err != nil {
			return TradeResult{}, fmt.Errorf("debit: %w", err)
		}
		if res == 0 {
			return TradeResult{}, ErrInsufficientFunds
		}
	} else {
		positionStr, err := qtx.GetPosition(ctx, database.GetPositionParams{
			UserID:  in.UserID,
			StockID: in.StockID,
		})
		if err != nil {
			return TradeResult{}, fmt.Errorf("get position: %w", err)
		}
		position, err := strconv.ParseFloat(positionStr, 64)
		if err != nil {
			return TradeResult{}, fmt.Errorf("parse position: %w", err)
		}
		// ponytail: float share compare, exact-decimal if it ever misfires
		if position < qty {
			return TradeResult{}, ErrInsufficientShares
		}
		if err := qtx.Credit(ctx, database.CreditParams{Cost: cost, UserID: in.UserID}); err != nil {
			return TradeResult{}, fmt.Errorf("credit: %w", err)
		}
	}

	trade, err := qtx.CreateTrade(ctx, database.CreateTradeParams{
		UserID:  in.UserID,
		StockID: in.StockID,
		Type:    in.Side,
		Qty:     in.Quantity, // exact string, no float
		Price:   closeAmount, // per-share fill price, cents
	})
	if err != nil {
		return TradeResult{}, fmt.Errorf("create trade: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TradeResult{}, fmt.Errorf("commit: %w", err)
	}

	return TradeResult{Trade: trade, Cost: cost}, nil
}

// houseCost returns the fill cost in cents, rounded in the house's favor:
// buys round up (user pays the extra sub-cent), sells round down. Pure.
func houseCost(closeCents int64, qty float64, side string) int64 {
	// ponytail: float ok, rounded to int cent on return
	raw := float64(closeCents) * qty
	if side == "buy" {
		return int64(math.Ceil(raw))
	}
	return int64(math.Floor(raw))
}
