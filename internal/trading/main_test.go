package trading

import (
	"testing"

	"github.com/t-011/mini_trading_platform/internal/exchange/exchangetest"
)

func TestMain(m *testing.M) {
	m.Run()
	exchangetest.RemoveBuild()
}
