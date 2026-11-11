package users

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

	"github.com/t-011/mini_trading_platform/internal/auth"
	"github.com/t-011/mini_trading_platform/internal/database"
)

// TestRefreshToken logs a user in against a real (migrated) Postgres and
// checks which refresh tokens still mint access tokens. Skips without DB_URL.
// Creates its own user and deletes it.
func TestRefreshToken(t *testing.T) {
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		t.Skip("DB_URL not set")
	}
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() }) // registered first so it runs last, after the row cleanup
	ctx := context.Background()

	const secret = "test-secret"
	service := New(database.New(conn), conn, secret)
	email := fmt.Sprintf("refresh-test-%s@example.com", uuid.NewString())
	user, err := service.Create(ctx, email, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// cascades the account and refresh tokens
		if _, err := conn.Exec(`DELETE FROM users WHERE id = $1`, user.ID); err != nil {
			t.Error(err)
		}
	})
	login := func() string {
		result, err := service.Login(ctx, email, "hunter2")
		if err != nil {
			t.Fatal(err)
		}
		return result.RefreshToken
	}

	// a fresh token gives an access token for the same user
	refreshToken := login()
	accessToken, err := service.Refresh(ctx, refreshToken)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if tokenUserID, err := auth.ValidateJWT(accessToken, secret); err != nil || tokenUserID != user.ID {
		t.Fatalf("access token is for %v (%v), want %v", tokenUserID, err, user.ID)
	}

	// a revoked token is refused
	if err := service.Revoke(ctx, refreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, refreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked token: got %v, want %v", err, ErrInvalidToken)
	}

	// an expired token is refused
	expiredToken := login()
	if _, err := conn.Exec(`UPDATE refresh_tokens SET expires_at = NOW() - INTERVAL '1 hour' WHERE token = $1`, expiredToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, expiredToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token: got %v, want %v", err, ErrInvalidToken)
	}

	// a token that was never issued is refused
	if _, err := service.Refresh(ctx, "never-issued"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unknown token: got %v, want %v", err, ErrInvalidToken)
	}
}
