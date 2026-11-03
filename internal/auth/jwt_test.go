package auth

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}
	if token == "" {
		t.Fatal("MakeJWT() returned an empty token")
	}

	validatedUserID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT() error = %v", err)
	}
	if validatedUserID != userID {
		t.Fatalf("ValidateJWT() user ID = %v, want %v", validatedUserID, userID)
	}
}

func TestValidateJWTRejectsExpiredToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, -time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	validatedUserID, err := ValidateJWT(token, secret)
	if err == nil {
		t.Fatal("ValidateJWT() error = nil, want an error for an expired token")
	}
	if validatedUserID != uuid.Nil {
		t.Fatalf("ValidateJWT() user ID = %v, want %v", validatedUserID, uuid.Nil)
	}
}

func TestValidateJWTRejectsWrongSecret(t *testing.T) {
	userID := uuid.New()

	token, err := MakeJWT(userID, "correct-secret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	validatedUserID, err := ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("ValidateJWT() error = nil, want an error for the wrong secret")
	}
	if validatedUserID != uuid.Nil {
		t.Fatalf("ValidateJWT() user ID = %v, want %v", validatedUserID, uuid.Nil)
	}
}

func TestGetBearerToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		fmt.Println(err)
	}

	wantToken := "1234"
	req.Header.Set("Authorization", "Bearer "+wantToken)

	token, err := GetBearerToken(req.Header)
	if err != nil {
		t.Fatalf("GetBearerToken() error getting token: %v", err.Error())
	}

	if token != "1234" {
		t.Fatalf("GetBearerToken() got token = %v, want = %v", token, wantToken)
	}
}

func TestMakeRefreshToken(t *testing.T) {
	token_1, _ := MakeRefreshToken()
	token_2, _ := MakeRefreshToken()

	if token_1 == token_2 {
		t.Fatalf("MakeRefreshToken(), duplicate keys")
	}

}
