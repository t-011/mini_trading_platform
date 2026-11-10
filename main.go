package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/t-011/mini_trading_platform/internal/api"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/exchange"
	"github.com/t-011/mini_trading_platform/internal/market"
	"github.com/t-011/mini_trading_platform/internal/trading"
	"github.com/t-011/mini_trading_platform/internal/users"
)

func main() {
	_ = godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	jwtSecret := os.Getenv("JWT_SECRET")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	queries := database.New(db)

	// Our exchange's stocks; the engine must already be serving them
	// (engine/me --serve MINI). Without it the rest of the API still works.
	exchanges := map[string]*exchange.Client{}
	for _, symbol := range []string{"MINI"} {
		client, err := exchange.Dial(symbol)
		if err != nil {
			log.Printf("exchange: %s unavailable, its orders are disabled: %v", symbol, err)
			continue
		}
		exchanges[symbol] = client
	}

	cfg := api.Config{
		JwtSecret: jwtSecret,

		Users:   users.New(queries, db, jwtSecret),
		Market:  market.New(queries),
		Trading: trading.New(queries, db, exchanges),
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:    ":" + port,
		Handler: api.NewHandler(&cfg),
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Error:", err)
	}
}
