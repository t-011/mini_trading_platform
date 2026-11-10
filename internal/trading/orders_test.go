package trading

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/exchange"
	"github.com/t-011/mini_trading_platform/internal/exchange/exchangetest"
)

// TestOrderFlow runs orders through a real engine and a real (migrated)
// Postgres, checking cash, shares and order state on both sides of a match.
// Skips without DB_URL. Creates its own users and stock and deletes them.
func TestOrderFlow(t *testing.T) {
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
	ctx := context.Background()

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
	newUser := func(cash int64) uuid.UUID {
		var userID uuid.UUID
		email := fmt.Sprintf("order-test-%s@example.com", uuid.NewString())
		if err := conn.QueryRow(`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO accounts (user_id, cash) VALUES ($1, $2)`, userID, cash); err != nil {
			t.Fatal(err)
		}
		return userID
	}
	seller := newUser(0)
	buyer := newUser(10_000)
	t.Cleanup(func() {
		// users first: cascades their accounts, orders and trades, which reference the stock
		if _, err := conn.Exec(`DELETE FROM users WHERE id IN ($1, $2)`, seller, buyer); err != nil {
			t.Error(err)
		}
		if _, err := conn.Exec(`DELETE FROM stocks WHERE id = $1`, stockID); err != nil {
			t.Error(err)
		}
	})
	// the seller already owns 10 shares
	if _, err := conn.Exec(`INSERT INTO trades (user_id, stock_id, type, qty, price) VALUES ($1, $2, 'buy', 10, 100)`,
		seller, stockID); err != nil {
		t.Fatal(err)
	}

	service := New(database.New(conn), conn, map[string]*exchange.Client{symbol: client})
	cashOf := func(userID uuid.UUID) int64 {
		cash, err := service.GetCash(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		return cash
	}
	place := func(userID uuid.UUID, side, qty string, price int64) (OrderResult, error) {
		return service.PlaceOrder(ctx, PlaceOrderInput{UserID: userID, StockID: stockID, Side: side, Quantity: qty, Price: price})
	}

	// seller rests 10 @ 150
	sell, err := place(seller, "sell", "10", 150)
	if err != nil || sell.Order.Status != "open" || len(sell.Trades) != 0 {
		t.Fatalf("rest sell: %+v %v", sell, err)
	}

	// buyer takes 4 with a 200 limit: fills at the maker's 150, gets 50/share back
	buy, err := place(buyer, "buy", "4", 200)
	if err != nil || buy.Order.Status != "filled" || len(buy.Trades) != 1 || buy.Trades[0].Price != 150 {
		t.Fatalf("crossing buy: %+v %v", buy, err)
	}
	if got := cashOf(buyer); got != 10_000-4*150 {
		t.Fatalf("buyer cash %d, want %d", got, 10_000-4*150)
	}
	if got := cashOf(seller); got != 4*150 {
		t.Fatalf("seller cash %d, want %d", got, 4*150)
	}

	// the 6 still resting are reserved: the seller can't sell 7 more of their 10
	if _, err := place(seller, "sell", "7", 150); !errors.Is(err, ErrInsufficientShares) {
		t.Fatalf("oversell: got %v", err)
	}
	if _, err := place(buyer, "buy", "100", 200); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("overspend: got %v", err)
	}
	if _, err := place(buyer, "buy", "1.5", 200); !errors.Is(err, ErrBadQuantity) {
		t.Fatalf("fractional: got %v", err)
	}

	// the buyer's portfolio values MINI-style stocks at the last trade price
	portfolio, err := service.GetPortfolio(ctx, buyer)
	if err != nil || len(portfolio.Holdings) != 1 || portfolio.Holdings[0].LastClose != 150 {
		t.Fatalf("portfolio: %+v %v", portfolio, err)
	}

	// seller cancels the remaining 6
	canceled, err := service.CancelOrder(ctx, seller, sell.Order.ID)
	if err != nil || canceled.Status != "canceled" || canceled.Filled != 4 || canceled.Canceled != 6 {
		t.Fatalf("cancel sell: %+v %v", canceled, err)
	}
	if _, err := service.CancelOrder(ctx, seller, sell.Order.ID); !errors.Is(err, ErrOrderNotOpen) {
		t.Fatalf("cancel twice: got %v", err)
	}

	// a resting buy holds its cash until cancelled, then gives it all back
	before := cashOf(buyer)
	resting, err := place(buyer, "buy", "2", 100)
	if err != nil || resting.Order.Status != "open" || cashOf(buyer) != before-200 {
		t.Fatalf("resting buy: %+v %v", resting, err)
	}
	if _, err := service.CancelOrder(ctx, seller, resting.Order.ID); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("cancel someone else's order: got %v", err)
	}
	if _, err := service.CancelOrder(ctx, buyer, resting.Order.ID); err != nil || cashOf(buyer) != before {
		t.Fatalf("cancel buy: %v, cash %d want %d", err, cashOf(buyer), before)
	}

	// read-only (non-exchange) stocks can't be traded
	var publicStock uuid.UUID
	if err := conn.QueryRow(`INSERT INTO stocks (symbol, name, exchange, asset_type, currency)
		VALUES ($1, 'Public Corp', 'NASDAQ', 'stock', 'USD') RETURNING id`, symbol+"P").Scan(&publicStock); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(`DELETE FROM stocks WHERE id = $1`, publicStock); err != nil {
			t.Error(err)
		}
	})
	_, err = service.PlaceOrder(ctx, PlaceOrderInput{UserID: buyer, StockID: publicStock, Side: "buy", Quantity: "1", Price: 100})
	if !errors.Is(err, ErrNotTradable) {
		t.Fatalf("public stock: got %v", err)
	}

}
