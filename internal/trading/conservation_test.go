package trading

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/exchange"
	"github.com/t-011/mini_trading_platform/internal/exchange/exchangetest"
)

// openTestDB connects to DB_URL, skipping the test without it.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		t.Skip("DB_URL not set")
	}
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() }) // registered first so it runs last, after the row cleanups
	return conn
}

// newTestStock serves a fresh symbol on its own engine and adds it to Postgres.
func newTestStock(t *testing.T, conn *sql.DB) (uuid.UUID, string, *exchange.Client) {
	t.Helper()
	symbol := exchangetest.StartEngine(t, exchange.RingDir)
	client, err := exchange.Dial(symbol)
	if err != nil {
		t.Fatal(err)
	}
	var stockID uuid.UUID
	err = conn.QueryRow(`INSERT INTO stocks (symbol, name, exchange, asset_type, currency)
		VALUES ($1, 'Test Corp', $2, 'stock', 'USD') RETURNING id`, symbol, ExchangeName).Scan(&stockID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(`DELETE FROM stocks WHERE id = $1`, stockID); err != nil {
			t.Error(err)
		}
	})
	return stockID, symbol, client
}

// newTestUser creates a user with cash and, for each stock, a free seed buy of shares.
func newTestUser(t *testing.T, conn *sql.DB, cash, shares int64, stockIDs ...uuid.UUID) uuid.UUID {
	t.Helper()
	var userID uuid.UUID
	email := fmt.Sprintf("conservation-test-%s@example.com", uuid.NewString())
	if err := conn.QueryRow(`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	// cascades their account, orders and trades; runs before the stock cleanups
	t.Cleanup(func() {
		if _, err := conn.Exec(`DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Error(err)
		}
	})
	if _, err := conn.Exec(`INSERT INTO accounts (user_id, cash) VALUES ($1, $2)`, userID, cash); err != nil {
		t.Fatal(err)
	}
	for _, stockID := range stockIDs {
		if _, err := conn.Exec(`INSERT INTO trades (user_id, stock_id, type, qty, price) VALUES ($1, $2, 'buy', $3, $4)`,
			userID, stockID, shares, seedPrice); err != nil {
			t.Fatal(err)
		}
	}
	return userID
}

const seedPrice = 100

// TestMoneyConservedUnderConcurrency has several users place random buys,
// sells and cancels on two stocks from many goroutines at once, against a
// real engine and a real Postgres. Once everything left open is canceled,
// no cash or shares may have been created or destroyed. Skips without DB_URL.
func TestMoneyConservedUnderConcurrency(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()

	const (
		userCount      = 6
		workersPerUser = 3
		opsPerWorker   = 40
		startingCash   = 2_000 // cents: a few orders' worth, so buys run out of funds
		startingShares = 5     // per stock, held by every other user, so sells run out too
	)

	exchanges := map[string]*exchange.Client{}
	var stockIDs []uuid.UUID
	for range 2 {
		stockID, symbol, client := newTestStock(t, conn)
		exchanges[symbol] = client
		stockIDs = append(stockIDs, stockID)
	}
	var userIDs []uuid.UUID
	holdsShares := map[uuid.UUID]bool{}
	for userIndex := range userCount {
		if userIndex%2 == 0 {
			userID := newTestUser(t, conn, startingCash, startingShares, stockIDs...)
			holdsShares[userID] = true
			userIDs = append(userIDs, userID)
		} else {
			userIDs = append(userIDs, newTestUser(t, conn, startingCash, 0))
		}
	}
	service := New(database.New(conn), conn, exchanges)

	// Each worker trades as one user with its own fixed seed, so a failing
	// run's order stream can be replayed (the interleaving can't). A worker
	// stops at its first unexpected error: if the engine died, every later
	// call would just wait out the engine timeout.
	var fundsRefused, sharesRefused atomic.Int64
	var workers sync.WaitGroup
	for workerIndex := range userCount * workersPerUser {
		userID := userIDs[workerIndex%userCount]
		workers.Go(func() {
			random := rand.New(rand.NewPCG(uint64(workerIndex), 0))
			var placed []int64
			for range opsPerWorker {
				if len(placed) > 0 && random.IntN(10) < 3 {
					orderID := placed[random.IntN(len(placed))]
					// already filled or canceled is fine, anything else is not
					if _, err := service.CancelOrder(ctx, userID, orderID); err != nil && !errors.Is(err, ErrOrderNotOpen) {
						t.Errorf("cancel order %d: %v", orderID, err)
						return
					}
					continue
				}
				side := "buy"
				if random.IntN(2) == 0 {
					side = "sell"
				}
				result, err := service.PlaceOrder(ctx, PlaceOrderInput{
					UserID:   userID,
					StockID:  stockIDs[random.IntN(len(stockIDs))],
					Side:     side,
					Quantity: strconv.Itoa(1 + random.IntN(5)),
					Price:    95 + random.Int64N(11), // narrow range: lots of crossing
				})
				switch {
				case errors.Is(err, ErrInsufficientFunds):
					fundsRefused.Add(1)
					continue
				case errors.Is(err, ErrInsufficientShares):
					sharesRefused.Add(1)
					continue
				case err != nil:
					t.Errorf("place %s: %v", side, err)
					return
				}
				placed = append(placed, result.Order.ID)
			}
		})
	}
	workers.Wait()
	if t.Failed() {
		return // a worker hit an unexpected error, the checks below would only time out too
	}

	queryInt := func(query string, args ...any) int64 {
		t.Helper()
		var value int64
		if err := conn.QueryRow(query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	// cancel everything still resting; with no traffic left each cancel must succeed
	var fillTrades int64
	for _, stockID := range stockIDs {
		rows, err := conn.Query(`SELECT id, user_id FROM orders WHERE stock_id = $1 AND status = 'open'`, stockID)
		if err != nil {
			t.Fatal(err)
		}
		openOrders := map[int64]uuid.UUID{}
		for rows.Next() {
			var orderID int64
			var userID uuid.UUID
			if err := rows.Scan(&orderID, &userID); err != nil {
				t.Fatal(err)
			}
			openOrders[orderID] = userID
		}
		rows.Close()
		for orderID, userID := range openOrders {
			if _, err := service.CancelOrder(ctx, userID, orderID); err != nil {
				t.Fatalf("final cancel of order %d: %v", orderID, err)
			}
		}

		// every order is closed: filled + canceled adds up to exactly its qty
		if unaccounted := queryInt(`SELECT COUNT(*) FROM orders WHERE stock_id = $1 AND filled + canceled <> qty`, stockID); unaccounted != 0 {
			t.Errorf("stock %s: %d orders not fully filled or canceled", stockID, unaccounted)
		}
		// fills move shares between users, so nobody can end up short
		shortUsers := queryInt(`SELECT COUNT(*) FROM (SELECT user_id FROM trades WHERE stock_id = $1
			GROUP BY user_id HAVING SUM(CASE WHEN type = 'buy' THEN qty ELSE -qty END) < 0) AS short`, stockID)
		if shortUsers != 0 {
			t.Errorf("stock %s: %d users hold a negative position", stockID, shortUsers)
		}
		fillTrades += queryInt(`SELECT COUNT(*) FROM trades WHERE stock_id = $1`, stockID) - int64(len(holdsShares))
	}

	// every cent moved is accounted for by the user's trades (the seed buys were free)
	for _, userID := range userIDs {
		cash, err := service.GetCash(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		tradeFlow := queryInt(`SELECT COALESCE(SUM(CASE WHEN type = 'sell' THEN qty * price ELSE -qty * price END), 0)::bigint
			FROM trades WHERE user_id = $1`, userID)
		if holdsShares[userID] {
			tradeFlow += int64(len(stockIDs)) * startingShares * seedPrice
		}
		if cash < 0 || cash != startingCash+tradeFlow {
			t.Errorf("user %s has cash %d, trades say %d", userID, cash, startingCash+tradeFlow)
		}
	}

	// the run actually traded and hit both risk checks, or the checks above prove little
	t.Logf("%d fill trades, %d buys refused for funds, %d sells refused for shares",
		fillTrades, fundsRefused.Load(), sharesRefused.Load())
	if fillTrades < 20 {
		t.Errorf("only %d fill trades, the run barely traded", fillTrades)
	}
	if fundsRefused.Load() == 0 || sharesRefused.Load() == 0 {
		t.Errorf("risk checks not exercised: %d funds refusals, %d shares refusals", fundsRefused.Load(), sharesRefused.Load())
	}
}

// TestConcurrentSellsCannotOversell fires 20 sells of a user's whole position
// at the same moment. The account lock lets exactly one through.
func TestConcurrentSellsCannotOversell(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	const sellers = 20

	stockID, symbol, client := newTestStock(t, conn)
	userID := newTestUser(t, conn, 0, 5, stockID)
	service := New(database.New(conn), conn, map[string]*exchange.Client{symbol: client})

	// open the connections up front so the sells race each other, not the dialing
	conn.SetMaxIdleConns(sellers)
	var warmConns []*sql.Conn
	for range sellers {
		warmConn, err := conn.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		warmConns = append(warmConns, warmConn)
	}
	for _, warmConn := range warmConns {
		warmConn.Close()
	}

	start := make(chan struct{})
	results := make(chan error, sellers)
	var workers sync.WaitGroup
	for range sellers {
		workers.Go(func() {
			<-start
			// 1000 is far above any bid, so the sell rests and keeps its shares reserved
			_, err := service.PlaceOrder(ctx, PlaceOrderInput{UserID: userID, StockID: stockID, Side: "sell", Quantity: "5", Price: 1000})
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)

	succeeded, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInsufficientShares):
			refused++
		default:
			t.Errorf("sell: %v", err)
		}
	}
	if succeeded != 1 || refused != sellers-1 {
		t.Fatalf("%d sells succeeded and %d were refused, want 1 and %d", succeeded, refused, sellers-1)
	}
}
