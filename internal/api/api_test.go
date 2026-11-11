package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/t-011/mini_trading_platform/internal/auth"
	"github.com/t-011/mini_trading_platform/internal/market"
	"github.com/t-011/mini_trading_platform/internal/trading"
	"github.com/t-011/mini_trading_platform/internal/users"
)

const testSecret = "test-secret"

// errorBody decodes the {"Error": "..."} body every error response carries.
func errorBody(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q is not JSON: %v", recorder.Body.String(), err)
	}
	return body.Error
}

func makeToken(t *testing.T, secret string, expiresIn time.Duration) string {
	t.Helper()
	token, err := auth.MakeJWT(uuid.New(), secret, expiresIn)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthMiddlewareRejects(t *testing.T) {
	cfg := &Config{JwtSecret: testSecret}

	tests := []struct {
		name          string
		authorization string
	}{
		{"no header", ""},
		{"not a bearer token", "Basic dXNlcjpwYXNz"},
		{"garbage token", "Bearer not-a-jwt"},
		{"wrong secret", "Bearer " + makeToken(t, "other-secret", time.Hour)},
		{"expired token", "Bearer " + makeToken(t, testSecret, -time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { nextCalled = true })
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			recorder := httptest.NewRecorder()
			cfg.middlewareAuth(next).ServeHTTP(recorder, request)

			if nextCalled {
				t.Fatal("request reached the handler")
			}
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
			if message := errorBody(t, recorder); message != "unauthorized" {
				t.Fatalf("error %q, want %q", message, "unauthorized")
			}
		})
	}
}

func TestAuthMiddlewarePassesUserID(t *testing.T) {
	cfg := &Config{JwtSecret: testSecret}
	userID := uuid.New()
	token, err := auth.MakeJWT(userID, testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var gotUserID uuid.UUID
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotUserID, _ = userIDFromContext(request.Context())
		writer.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	cfg.middlewareAuth(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent || gotUserID != userID {
		t.Fatalf("status %d, user %v, want %d and %v", recorder.Code, gotUserID, http.StatusNoContent, userID)
	}
}

// These handlers decode the body before touching a service, so a nil
// service is never reached.
func TestMalformedJSONBody(t *testing.T) {
	handler := NewHandler(&Config{JwtSecret: testSecret})

	tests := []struct {
		name   string
		method string
		path   string
		authed bool
	}{
		{"signup", http.MethodPost, "/v1/users", false},
		{"login", http.MethodPost, "/v1/login", false},
		{"place order", http.MethodPost, "/v1/orders", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"Email": `))
			if test.authed {
				request.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, time.Hour))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if message := errorBody(t, recorder); message != "invalid request body" {
				t.Fatalf("error %q, want %q", message, "invalid request body")
			}
		})
	}
}

func TestRespondWithServiceError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{trading.ErrBadSide, http.StatusBadRequest},
		{trading.ErrBadQuantity, http.StatusBadRequest},
		{trading.ErrBadNote, http.StatusBadRequest},
		{trading.ErrBadPrice, http.StatusBadRequest},
		{market.ErrBadInterval, http.StatusBadRequest},
		{users.ErrMissingFields, http.StatusBadRequest},
		{users.ErrInvalidCredentials, http.StatusUnauthorized},
		{users.ErrInvalidToken, http.StatusUnauthorized},
		{users.ErrEmailTaken, http.StatusConflict},
		{trading.ErrTradeNotFound, http.StatusNotFound},
		{trading.ErrStockNotFound, http.StatusNotFound},
		{trading.ErrOrderNotFound, http.StatusNotFound},
		{market.ErrStockNotFound, http.StatusNotFound},
		{trading.ErrInsufficientFunds, http.StatusUnprocessableEntity},
		{trading.ErrInsufficientShares, http.StatusUnprocessableEntity},
		{trading.ErrNotTradable, http.StatusUnprocessableEntity},
		{trading.ErrOrderNotOpen, http.StatusConflict},
		{trading.ErrExchangeDown, http.StatusServiceUnavailable},
		{fmt.Errorf("place order: %w", trading.ErrInsufficientFunds), http.StatusUnprocessableEntity}, // wrapped
	}
	for _, test := range tests {
		t.Run(test.err.Error(), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			respondWithServiceError(recorder, test.err)

			if recorder.Code != test.status {
				t.Fatalf("status %d, want %d", recorder.Code, test.status)
			}
			if message := errorBody(t, recorder); message != test.err.Error() {
				t.Fatalf("error %q, want %q", message, test.err.Error())
			}
		})
	}

	// unknown errors are a 500 that doesn't leak the internal message
	recorder := httptest.NewRecorder()
	respondWithServiceError(recorder, errors.New("connection refused"))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unknown error: status %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if message := errorBody(t, recorder); message != "" {
		t.Fatalf("unknown error leaked %q", message)
	}
}
