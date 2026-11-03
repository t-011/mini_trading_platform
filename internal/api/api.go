package api

import (
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/database"
)

type Config struct {
	Db             *database.Queries
	JwtSecret 	   string
}

func NewHandler(cfg *Config) http.Handler {
	mux := http.NewServeMux()

	return mux
}
