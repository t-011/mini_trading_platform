package market

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/t-011/mini_trading_platform/internal/database"
)

// validInterval whitelists the date_trunc buckets we expose. Extend here (and
// accept narrower from/to windows) if we ever add intraday intervals.
var validInterval = map[string]bool{"day": true, "week": true, "month": true}

type GetBarsInput struct {
	StockID  uuid.UUID
	From     string // "YYYY-MM-DD"; empty -> default window
	To       string // "YYYY-MM-DD"; empty -> now
	Interval string // must be in validInterval
}

// GetBars returns aggregated bars for a stock over a date window. It owns the
// interval whitelist and the default 30-day window.
func (s *Service) GetBars(ctx context.Context, in GetBarsInput) ([]database.GetBarsByIntervalRow, error) {
	if !validInterval[in.Interval] {
		return nil, ErrBadInterval
	}

	to := parseDate(in.To, time.Now())
	from := parseDate(in.From, to.AddDate(0, 0, -30)) // default: last 30 days

	return s.db.GetBarsByInterval(ctx, database.GetBarsByIntervalParams{
		Interval:      in.Interval,
		StockID:       in.StockID,
		TimestampFrom: from,
		TimestampTo:   to,
	})
}

func parseDate(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", s) // Go's reference date
	if err != nil {
		return def
	}
	return t
}
