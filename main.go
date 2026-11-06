package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/t-011/mini_trading_platform/internal/api"
	"github.com/t-011/mini_trading_platform/internal/database"
	"github.com/t-011/mini_trading_platform/internal/trading"
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
	cfg := api.Config{
		Db:        queries,
		DbConn:    db,
		JwtSecret: jwtSecret,

		Trading: trading.New(queries, db),
	}

	server := &http.Server{
		Addr:    ":8080",
		Handler: api.NewHandler(&cfg),
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Error:", err)
	}
}
