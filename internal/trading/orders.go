package trading

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/exchange"
)

// ExchangeName marks the stocks traded on our own exchange (stocks.exchange).
// Every other stock is read-only.
const ExchangeName = "MINI_EXCHANGE"

const (
	unitsPerShare = 100_000_000     // the engine counts qty in 1e-8 shares
	engineTimeout = 5 * time.Second // per round trip to the engine
)

type PlaceOrderInput struct {
	UserID   uuid.UUID
	StockID  uuid.UUID
	Side     string // "buy" or "sell"
	Quantity string // whole shares
	Price    int64  // limit price, cents
}

type OrderResult struct {
	Order  database.Order
	Trades []database.Trade // the caller's own fills from this order
}

// PlaceOrder sends a limit order to the exchange. Money flow:
//   - buy: limit*qty is debited up front (the reservation). Each fill refunds
//     the price improvement, a cancel refunds limit*cancelled.
//   - sell: the shares are reserved (open sell orders count against the
//     position). Each fill pays the seller the trade price.
//
// Every one of those updates is relative (filled += n, cash += n), so the
// order in which concurrent answers reach the DB doesn't matter.
func (s *Service) PlaceOrder(ctx context.Context, in PlaceOrderInput) (OrderResult, error) {
	if in.Side != "buy" && in.Side != "sell" {
		return OrderResult{}, ErrBadSide
	}
	shares, err := strconv.ParseInt(in.Quantity, 10, 64)
	if err != nil || shares <= 0 || shares > math.MaxInt64/unitsPerShare {
		return OrderResult{}, ErrBadQuantity
	}
	if in.Price <= 0 || in.Price > math.MaxInt64/shares {
		return OrderResult{}, ErrBadPrice
	}

	client, err := s.clientFor(ctx, in.StockID)
	if err != nil {
		return OrderResult{}, err
	}

	order, err := s.reserveAndRecord(ctx, in, shares)
	if err != nil {
		return OrderResult{}, err
	}

	// The order is committed and about to hit the engine: finish recording the
	// answer even if the HTTP client hangs up.
	engineCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), engineTimeout)
	defer cancel()
	events, err := client.Submit(engineCtx, uint64(order.ID), in.Side == "buy", in.Price, shares*unitsPerShare)
	if err != nil {
		// ponytail: the order stays open with its reservation held; reconcile against the engine once the WAL exists
		return OrderResult{}, fmt.Errorf("submit order %d: %w", order.ID, err)
	}
	return s.applyAnswer(engineCtx, order, events)
}

// CancelOrder pulls the rest of one of the caller's open orders off the book.
func (s *Service) CancelOrder(ctx context.Context, userID uuid.UUID, orderID int64) (database.Order, error) {
	order, err := s.db.GetOrder(ctx, orderID)
	if err == sql.ErrNoRows || (err == nil && order.UserID != userID) {
		return database.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return database.Order{}, err
	}
	if order.Status != "open" {
		return database.Order{}, ErrOrderNotOpen
	}

	client, err := s.clientFor(ctx, order.StockID)
	if err != nil {
		return database.Order{}, err
	}

	engineCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), engineTimeout)
	defer cancel()
	events, err := client.Cancel(engineCtx, uint64(order.ID))
	if err != nil {
		return database.Order{}, fmt.Errorf("cancel order %d: %w", order.ID, err)
	}
	answer := events[len(events)-1]
	if answer.Type != exchange.EvCanceled {
		return database.Order{}, ErrOrderNotOpen // filled in the meantime
	}
	removedShares := answer.Qty / unitsPerShare

	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	tx, err := s.conn.BeginTx(engineCtx, nil)
	if err != nil {
		return database.Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.db.WithTx(tx)

	canceled, err := qtx.CancelOrder(engineCtx, database.CancelOrderParams{Shares: removedShares, ID: order.ID})
	if err != nil {
		return database.Order{}, fmt.Errorf("cancel order row: %w", err)
	}
	if order.Side == "buy" {
		refund := database.CreditParams{Cost: order.Price * removedShares, UserID: order.UserID}
		if err := qtx.Credit(engineCtx, refund); err != nil {
			return database.Order{}, fmt.Errorf("refund: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return database.Order{}, fmt.Errorf("commit: %w", err)
	}
	return canceled, nil
}

func (s *Service) ListOrders(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]database.Order, error) {
	return s.db.ListOrdersByUser(ctx, database.ListOrdersByUserParams{UserID: userID, Limit: limit, Offset: offset})
}

// clientFor returns the engine connection for a stock on our exchange.
func (s *Service) clientFor(ctx context.Context, stockID uuid.UUID) (*exchange.Client, error) {
	stock, err := s.db.GetStockById(ctx, stockID)
	if err == sql.ErrNoRows {
		return nil, ErrStockNotFound
	}
	if err != nil {
		return nil, err
	}
	if stock.Exchange != ExchangeName {
		return nil, ErrNotTradable
	}
	client := s.exchanges[stock.Symbol]
	if client == nil {
		return nil, ErrExchangeDown
	}
	return client, nil
}

// reserveAndRecord checks and reserves the cash or shares, then stores the
// open order, all under the account lock so a user's orders serialize.
func (s *Service) reserveAndRecord(ctx context.Context, in PlaceOrderInput, shares int64) (database.Order, error) {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return database.Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.db.WithTx(tx)

	if err := qtx.LockAccount(ctx, in.UserID); err != nil {
		return database.Order{}, fmt.Errorf("lock account: %w", err)
	}

	if in.Side == "buy" {
		debited, err := qtx.Debit(ctx, database.DebitParams{Cost: in.Price * shares, UserID: in.UserID})
		if err != nil {
			return database.Order{}, fmt.Errorf("debit: %w", err)
		}
		if debited == 0 {
			return database.Order{}, ErrInsufficientFunds
		}
	} else {
		positionText, err := qtx.GetPosition(ctx, database.GetPositionParams{UserID: in.UserID, StockID: in.StockID})
		if err != nil {
			return database.Order{}, fmt.Errorf("get position: %w", err)
		}
		position, err := strconv.ParseFloat(positionText, 64)
		if err != nil {
			return database.Order{}, fmt.Errorf("parse position: %w", err)
		}
		reserved, err := qtx.GetReservedShares(ctx, database.GetReservedSharesParams{UserID: in.UserID, StockID: in.StockID})
		if err != nil {
			return database.Order{}, fmt.Errorf("reserved shares: %w", err)
		}
		if position-float64(reserved) < float64(shares) {
			return database.Order{}, ErrInsufficientShares
		}
	}

	order, err := qtx.CreateOrder(ctx, database.CreateOrderParams{
		UserID:  in.UserID,
		StockID: in.StockID,
		Side:    in.Side,
		Price:   in.Price,
		Qty:     shares,
	})
	if err != nil {
		return database.Order{}, fmt.Errorf("create order: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return database.Order{}, fmt.Errorf("commit: %w", err)
	}
	return order, nil
}

// applyAnswer records the engine's answer to a new order in one transaction.
// applyMu serializes these: answers touch other users' orders and accounts
// (the makers), and two transactions locking those rows in different orders
// would deadlock.
// ponytail: one lock for every stock, per-stock locks if throughput ever matters
func (s *Service) applyAnswer(ctx context.Context, order database.Order, events []exchange.Event) (OrderResult, error) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return OrderResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.db.WithTx(tx)

	result := OrderResult{Trades: []database.Trade{}}
	for _, event := range events {
		switch event.Type {
		case exchange.EvFill:
			takerTrade, err := applyFill(ctx, qtx, order.StockID, event)
			if err != nil {
				return OrderResult{}, err
			}
			result.Trades = append(result.Trades, takerTrade)
		case exchange.EvRejected:
			// The engine rejects before matching, so nothing was filled.
			if err := qtx.RejectOrder(ctx, order.ID); err != nil {
				return OrderResult{}, fmt.Errorf("reject order: %w", err)
			}
			if order.Side == "buy" {
				refund := database.CreditParams{Cost: order.Price * order.Qty, UserID: order.UserID}
				if err := qtx.Credit(ctx, refund); err != nil {
					return OrderResult{}, fmt.Errorf("refund: %w", err)
				}
			}
		}
	}

	if result.Order, err = qtx.GetOrder(ctx, order.ID); err != nil {
		return OrderResult{}, fmt.Errorf("reload order: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return OrderResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// applyFill records one match for both orders in it (the taker's and the
// resting maker's): a trade row each, their filled count, and the cash.
// Returns the taker's trade.
func applyFill(ctx context.Context, qtx *database.Queries, stockID uuid.UUID, fill exchange.Event) (database.Trade, error) {
	shares := fill.Qty / unitsPerShare
	var takerTrade database.Trade
	for _, orderID := range []uint64{fill.OrderID, fill.MakerID} {
		filledOrder, err := qtx.FillOrder(ctx, database.FillOrderParams{Shares: shares, ID: int64(orderID)})
		if err != nil {
			return database.Trade{}, fmt.Errorf("fill order %d: %w", orderID, err)
		}
		trade, err := qtx.CreateTrade(ctx, database.CreateTradeParams{
			UserID:  filledOrder.UserID,
			StockID: stockID,
			Type:    filledOrder.Side,
			Qty:     strconv.FormatInt(shares, 10),
			Price:   fill.Price,
		})
		if err != nil {
			return database.Trade{}, fmt.Errorf("record trade: %w", err)
		}

		credit := fill.Price * shares // the seller is paid the trade price
		if filledOrder.Side == "buy" {
			credit = (filledOrder.Price - fill.Price) * shares // buyer reserved the limit, gets the difference back
		}
		if credit > 0 {
			if err := qtx.Credit(ctx, database.CreditParams{Cost: credit, UserID: filledOrder.UserID}); err != nil {
				return database.Trade{}, fmt.Errorf("credit: %w", err)
			}
		}

		if orderID == fill.OrderID {
			takerTrade = trade
		}
	}
	return takerTrade, nil
}
