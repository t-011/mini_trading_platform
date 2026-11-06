// Package users holds the account/auth domain: signup (user + cash account in
// one tx), login, profile update, and refresh-token lifecycle. Token minting
// lives here; token validation for requests stays in the api middleware.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/t-011/mini_trading_platform/internal/auth"
	"github.com/t-011/mini_trading_platform/internal/database"
)

const (
	accessTokenDuration  = 3600 * time.Second  // 1h
	refreshTokenDuration = 60 * 24 * time.Hour // 60 days
)

type Service struct {
	db        *database.Queries
	conn      *sql.DB // raw pool, for the signup transaction
	jwtSecret string
}

func New(db *database.Queries, conn *sql.DB, jwtSecret string) *Service {
	return &Service{db: db, conn: conn, jwtSecret: jwtSecret}
}

type LoginResult struct {
	User         database.User
	AccessToken  string
	RefreshToken string
}

// Create registers a user and opens their cash account in one transaction.
func (s *Service) Create(ctx context.Context, email, password string) (database.User, error) {
	if email == "" || password == "" {
		return database.User{}, ErrMissingFields
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		return database.User{}, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return database.User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.db.WithTx(tx)

	user, err := qtx.CreateUser(ctx, database.CreateUserParams{
		Email:          email,
		HashedPassword: hashed,
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
			return database.User{}, ErrEmailTaken
		}
		return database.User{}, fmt.Errorf("create user: %w", err)
	}

	if err := qtx.CreateAccountForUserId(ctx, user.ID); err != nil {
		return database.User{}, fmt.Errorf("create account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return database.User{}, fmt.Errorf("commit: %w", err)
	}

	return user, nil
}

// Login verifies credentials and mints an access + refresh token pair.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	if email == "" || password == "" {
		return LoginResult{}, ErrMissingFields
	}

	user, err := s.db.GetUserByEmail(ctx, email)
	if err == sql.ErrNoRows {
		return LoginResult{}, ErrInvalidCredentials // don't leak "no such email"
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("get user: %w", err)
	}

	ok, err := auth.CheckPasswordHash(password, user.HashedPassword)
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}

	accessToken, err := auth.MakeJWT(user.ID, s.jwtSecret, accessTokenDuration)
	if err != nil {
		return LoginResult{}, fmt.Errorf("make jwt: %w", err)
	}

	refreshStr, err := auth.MakeRefreshToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("make refresh token: %w", err)
	}

	refreshToken, err := s.db.CreateRefreshToken(ctx, database.CreateRefreshTokenParams{
		Token:     refreshStr,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(refreshTokenDuration),
		RevokedAt: sql.NullTime{},
	})
	if err != nil {
		return LoginResult{}, fmt.Errorf("create refresh token: %w", err)
	}

	return LoginResult{User: user, AccessToken: accessToken, RefreshToken: refreshToken.Token}, nil
}

// Update changes the authenticated user's email/password.
func (s *Service) Update(ctx context.Context, userID uuid.UUID, email, password string) (database.User, error) {
	if email == "" || password == "" {
		return database.User{}, ErrMissingFields
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		return database.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.db.UpdateUser(ctx, database.UpdateUserParams{
		ID:             userID,
		Email:          email,
		HashedPassword: hashed,
	})
	if err != nil {
		return database.User{}, fmt.Errorf("update user: %w", err)
	}

	return user, nil
}

// Refresh exchanges a valid, unexpired, unrevoked refresh token for a new access token.
func (s *Service) Refresh(ctx context.Context, token string) (string, error) {
	refreshToken, err := s.db.GetRefreshToken(ctx, token)
	if err == sql.ErrNoRows {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("get refresh token: %w", err)
	}

	if time.Now().After(refreshToken.ExpiresAt) || refreshToken.RevokedAt.Valid {
		return "", ErrInvalidToken
	}

	accessToken, err := auth.MakeJWT(refreshToken.UserID, s.jwtSecret, accessTokenDuration)
	if err != nil {
		return "", fmt.Errorf("make jwt: %w", err)
	}

	return accessToken, nil
}

// Revoke marks a refresh token as revoked.
func (s *Service) Revoke(ctx context.Context, token string) error {
	if err := s.db.RevokeToken(ctx, token); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}
