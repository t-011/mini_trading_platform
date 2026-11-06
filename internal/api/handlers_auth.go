package api

import (
	"encoding/json"
	"net/http"

	"github.com/t-011/mini_trading_platform/internal/auth"
)

type authParams struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

func (cfg *Config) createUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	var reqData authParams
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := cfg.Users.Create(r.Context(), reqData.Email, reqData.Password)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Email      string `json:"email"`
	}

	respondWithJSON(w, http.StatusCreated, respBody{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt.String(),
		Updated_at: user.UpdatedAt.String(),
		Email:      user.Email,
	})
}

func (cfg *Config) loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	var reqData authParams
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := cfg.Users.Login(r.Context(), reqData.Email, reqData.Password)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Id           string `json:"id"`
		Created_at   string `json:"created_at"`
		Updated_at   string `json:"updated_at"`
		Email        string `json:"email"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	respondWithJSON(w, http.StatusOK, respBody{
		Id:           result.User.ID.String(),
		Created_at:   result.User.CreatedAt.String(),
		Updated_at:   result.User.UpdatedAt.String(),
		Email:        result.User.Email,
		Token:        result.AccessToken,
		RefreshToken: result.RefreshToken,
	})
}

func (cfg *Config) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	var reqData authParams
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := cfg.Users.Update(r.Context(), userID, reqData.Email, reqData.Password)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Email      string `json:"email"`
	}

	respondWithJSON(w, http.StatusOK, respBody{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt.String(),
		Updated_at: user.UpdatedAt.String(),
		Email:      user.Email,
	})
}

func (cfg *Config) refreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	accessToken, err := cfg.Users.Refresh(r.Context(), token)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	type respBody struct {
		Token string `json:"token"`
	}

	respondWithJSON(w, http.StatusOK, respBody{Token: accessToken})
}

func (cfg *Config) revokeTokenHandler(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := cfg.Users.Revoke(r.Context(), token); err != nil {
		respondWithServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
