package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/t-011/mini_trading_platform/internal/auth"
	"github.com/t-011/mini_trading_platform/internal/database"
)

const (
	ACCESS_TOKEN_DURATION  = 3600 * time.Second  // 1hr
	REFRESH_TOKEN_DURATION = 60 * 24 * time.Hour // 60 days
)

func (cfg *Config) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	w.Header().Set("Content-type", "application/json")

	var reqData params
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode json")
		return
	}

	if reqData.Password == "" || reqData.Email == "" {
		respondWithError(w, http.StatusBadRequest, "Required fields missing")
		return
	}

	hashed_password, err := auth.HashPassword(reqData.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	user, err := cfg.Db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          reqData.Email,
		HashedPassword: hashed_password,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "DB ERROR: "+err.Error())
		return
	}

	type respBody struct {
		Id         string `json:"id"`
		Created_at string `json:"created_at"`
		Updated_at string `json:"updated_at"`
		Email      string `json:"email"`
	}

	payload := respBody{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt.String(),
		Updated_at: user.UpdatedAt.String(),
		Email:      user.Email,
	}

	respondWithJSON(w, http.StatusCreated, payload)
}

func (cfg *Config) loginHandler(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	w.Header().Set("Content-type", "application/json")

	var reqData params
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode json")
		return
	}

	if reqData.Password == "" || reqData.Email == "" {
		respondWithError(w, http.StatusBadRequest, "Required fields missing")
		return
	}

	user, err := cfg.Db.GetUserByEmail(r.Context(), reqData.Email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "DB ERROR: "+err.Error())
		return
	}

	ok, err := auth.CheckPasswordHash(reqData.Password, user.HashedPassword)
	if err != nil || !ok {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
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

	accessToken, err := auth.MakeJWT(user.ID, cfg.JwtSecret, ACCESS_TOKEN_DURATION)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Server Error")
		return
	}

	newRefreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Server Error")
		return
	}

	refreshToken, err := cfg.Db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     newRefreshToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(REFRESH_TOKEN_DURATION),
		RevokedAt: sql.NullTime{},
	})
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Server Error")
		return
	}

	payload := respBody{
		Id:           user.ID.String(),
		Created_at:   user.CreatedAt.String(),
		Updated_at:   user.UpdatedAt.String(),
		Email:        user.Email,
		Token:        accessToken,
		RefreshToken: refreshToken.Token,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func (cfg *Config) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	w.Header().Set("Content-type", "application/json")

	var reqData params
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode json")
		return
	}

	if reqData.Password == "" || reqData.Email == "" {
		respondWithError(w, http.StatusBadRequest, "Required fields missing")
		return
	}

	// Get userid from jwt
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	hashed_password, err := auth.HashPassword(reqData.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	user, err := cfg.Db.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:             userID,
		Email:          reqData.Email,
		HashedPassword: hashed_password,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "")
		return
	}

	type respBody struct {
		Id          string `json:"id"`
		Created_at  string `json:"created_at"`
		Updated_at  string `json:"updated_at"`
		Email       string `json:"email"`
	}

	payload := respBody{
		Id:          user.ID.String(),
		Created_at:  user.CreatedAt.String(),
		Updated_at:  user.UpdatedAt.String(),
		Email:       user.Email,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func (cfg *Config) refreshTokenHandler(w http.ResponseWriter, r *http.Request) {

	// Get refresh token from header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	refreshToken, err := cfg.Db.GetRefreshToken(r.Context(), token)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		} else {
			fmt.Println(err)
			respondWithError(w, http.StatusInternalServerError, "Server Error")
		}
		return
	}

	if time.Now().After(refreshToken.ExpiresAt) || refreshToken.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	type respBody struct {
		Token string `json:"token"`
	}

	jwtToken, err := auth.MakeJWT(refreshToken.UserID, cfg.JwtSecret, ACCESS_TOKEN_DURATION)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Server Error")
		return
	}

	payload := respBody{
		Token: jwtToken,
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func (cfg *Config) revokeTokenHandler(w http.ResponseWriter, r *http.Request) {

	// Get refresh token from header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	err = cfg.Db.RevokeToken(r.Context(), token)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Server Error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
