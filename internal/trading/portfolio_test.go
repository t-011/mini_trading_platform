package trading

import (
	"testing"

	"github.com/t-011/mini_trading_platform/internal/database"
)

func TestDeriveHolding(t *testing.T) {
	// 2 shares bought avg 150.00 (15000c), now 160.00 (16000c):
	// market_value = 32000, cost_basis = 30000, unrealized = +2000.
	h := deriveHolding(database.GetHoldingsRow{
		Qty: "2", AvgCost: "15000", LastClose: 16000,
	})
	if h.MarketValue != 32000 || h.CostBasis != 30000 || h.Unrealized != 2000 {
		t.Fatalf("whole shares: got mv=%d cb=%d pnl=%d", h.MarketValue, h.CostBasis, h.Unrealized)
	}

	// Fractional qty with sub-cent products must round to the nearest cent.
	// 0.5 * 10001 = 5000.5 -> 5001 (market value), 0.5 * 9999 = 4999.5 -> 5000.
	f := deriveHolding(database.GetHoldingsRow{
		Qty: "0.5", AvgCost: "9999", LastClose: 10001,
	})
	if f.MarketValue != 5001 || f.CostBasis != 5000 || f.Unrealized != 1 {
		t.Fatalf("fractional: got mv=%d cb=%d pnl=%d", f.MarketValue, f.CostBasis, f.Unrealized)
	}
}
