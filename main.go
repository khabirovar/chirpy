package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/khabirovar/chirpy/internal/auth"
	"github.com/khabirovar/chirpy/internal/database"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	tokenSecret    string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type UserStruct struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	godotenv.Load()
	dbUrl := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	apiCfg := apiConfig{
		db:          database.New(db),
		platform:    os.Getenv("PLATFORM"),
		tokenSecret: os.Getenv("TOKEN_SECRET"),
	}

	mux.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetrics(http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	metricsPage := `
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
`
	mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		msg := fmt.Sprintf(metricsPage, apiCfg.fileserverHits.Load())
		w.Write([]byte(msg))
	})

	mux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if apiCfg.platform != "dev" {
			respondWithError(w, http.StatusForbidden, "Forbidden")
			return
		}

		err := apiCfg.db.DeleteUsers(r.Context())
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		apiCfg.fileserverHits.Swap(int32(0))
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		userID, err := auth.ValidateJWT(token, apiCfg.tokenSecret)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}
		fmt.Printf("user: %v\n", userID)

		type RespBody struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
		}

		body := RespBody{}
		err = json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Something went wrong")
			return
		}
		body.UserID = userID

		if len(body.Body) > 140 {
			respondWithError(w, http.StatusBadRequest, "Chirp is too long")
			return
		}

		dirty := map[string]bool{
			"kerfuffle": true,
			"sharbert":  true,
			"fornax":    true,
		}

		words := strings.Split(body.Body, " ")
		cleaned := make([]string, 0, len(words))
		for _, word := range words {
			if _, isDirty := dirty[strings.ToLower(word)]; isDirty {
				cleaned = append(cleaned, "****")
			} else {
				cleaned = append(cleaned, word)
			}
		}

		params := database.CreateChirpParams{
			Body:   body.Body,
			UserID: body.UserID,
		}
		chirpFromDB, err := apiCfg.db.CreateChirp(r.Context(), params)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		chirp := Chirp{
			ID:        chirpFromDB.ID,
			CreatedAt: chirpFromDB.CreatedAt,
			UpdatedAt: chirpFromDB.UpdatedAt,
			Body:      chirpFromDB.Body,
			UserID:    chirpFromDB.UserID,
		}
		respondWithJSON(w, http.StatusCreated, chirp)
	})

	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		var paramsJson UserStruct
		err := json.NewDecoder(r.Body).Decode(&paramsJson)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		params := database.CreateUserParams{
			Email: paramsJson.Email,
		}
		params.HashedPassword, err = auth.HashPassword(paramsJson.Password)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var user User
		userFromDB, err := apiCfg.db.CreateUser(r.Context(), params)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		user.ID = userFromDB.ID
		user.CreatedAt = userFromDB.CreatedAt
		user.UpdatedAt = userFromDB.UpdatedAt
		user.Email = userFromDB.Email

		respondWithJSON(w, http.StatusCreated, user)
	})

	mux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		userIDFromToken, err := auth.ValidateJWT(bearerToken, apiCfg.tokenSecret)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		var userJson UserStruct
		err = json.NewDecoder(r.Body).Decode(&userJson)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		hashedPassword, err := auth.HashPassword(userJson.Password)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		userUpdateParams := database.UpdateUserParams{
			ID:             userIDFromToken,
			Email:          userJson.Email,
			HashedPassword: hashedPassword,
		}
		userFromDB, err := apiCfg.db.UpdateUser(r.Context(), userUpdateParams)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var user User
		user.ID = userFromDB.ID
		user.CreatedAt = userFromDB.CreatedAt
		user.UpdatedAt = userFromDB.UpdatedAt
		user.Email = userFromDB.Email
		respondWithJSON(w, http.StatusOK, user)
	})

	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		type Login struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		var login Login
		err := json.NewDecoder(r.Body).Decode(&login)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		userFromDB, err := apiCfg.db.GetUserByEmail(r.Context(), login.Email)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		match, err := auth.CheckPasswordHash(login.Password, userFromDB.HashedPassword)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !match {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		token, err := auth.MakeJWT(userFromDB.ID, apiCfg.tokenSecret, time.Hour)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		refreshToken := auth.MakeRefreshToken()
		paramsRefreshToken := database.CreateRefreshTokenParams{
			Token:     refreshToken,
			UserID:    userFromDB.ID,
			ExpiresAt: time.Now().AddDate(0, 0, 60),
		}
		err = apiCfg.db.CreateRefreshToken(r.Context(), paramsRefreshToken)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		user := User{
			ID:           userFromDB.ID,
			CreatedAt:    userFromDB.CreatedAt,
			UpdatedAt:    userFromDB.UpdatedAt,
			Email:        userFromDB.Email,
			Token:        token,
			RefreshToken: refreshToken,
		}
		respondWithJSON(w, http.StatusOK, user)
	})

	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		refreshTokenDB, err := apiCfg.db.GetUserFromRefreshToken(r.Context(), bearerToken)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}
		now := time.Now().UTC()
		if now.After(refreshTokenDB.ExpiresAt) || refreshTokenDB.RevokedAt.Valid { //now.After(refreshTokenDB.RevokedAt.Time) {
			respondWithError(w, http.StatusUnauthorized, "Timestamps are expired")
			return
		}

		token, err := auth.MakeJWT(refreshTokenDB.UserID, apiCfg.tokenSecret, time.Hour)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}
		respondWithJSON(w, http.StatusOK, map[string]string{"token": token})
	})

	mux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		err = apiCfg.db.RevokeRefreshToken(r.Context(), bearerToken)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		chirpsFromDB, err := apiCfg.db.GetChirps(r.Context())
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		chirps := make([]Chirp, len(chirpsFromDB))
		for idx := range chirpsFromDB {
			chirps[idx].ID = chirpsFromDB[idx].ID
			chirps[idx].CreatedAt = chirpsFromDB[idx].CreatedAt
			chirps[idx].UpdatedAt = chirpsFromDB[idx].UpdatedAt
			chirps[idx].Body = chirpsFromDB[idx].Body
			chirps[idx].UserID = chirpsFromDB[idx].UserID
		}
		respondWithJSON(w, http.StatusOK, chirps)
	})

	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		chirpIDString := r.PathValue("chirpID")
		if chirpIDString == "" {
			respondWithError(w, http.StatusBadRequest, "Wrong chirp ID")
			return
		}

		chirpID, err := uuid.Parse(chirpIDString)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		chirpFromDB, err := apiCfg.db.GetChirpByID(r.Context(), chirpID)
		if err != nil {
			respondWithError(w, http.StatusNotFound, err.Error())
			return
		}
		chirp := Chirp{
			ID:        chirpFromDB.ID,
			CreatedAt: chirpFromDB.CreatedAt,
			UpdatedAt: chirpFromDB.UpdatedAt,
			Body:      chirpFromDB.Body,
			UserID:    chirpFromDB.UserID,
		}
		respondWithJSON(w, http.StatusOK, chirp)
	})

	mux.HandleFunc("DELETE /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}
		userID, err := auth.ValidateJWT(bearerToken, apiCfg.tokenSecret)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}

		chirpIDString := r.PathValue("chirpID")
		if chirpIDString == "" {
			respondWithError(w, http.StatusNotFound, "Wrong chirp id")
			return
		}
		chirpID, err := uuid.Parse(chirpIDString)
		if err != nil {
			respondWithError(w, http.StatusNotFound, err.Error())
			return
		}
		chirp, err := apiCfg.db.GetChirpByID(r.Context(), chirpID)
		if err != nil {
			respondWithError(w, http.StatusNotFound, err.Error())
			return
		}

		if userID != chirp.UserID {
			respondWithError(w, http.StatusForbidden, "User not own this chirp")
			return 
		}

		err = apiCfg.db.DeleteChirpByID(r.Context(), chirp.ID)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	log.Fatal(server.ListenAndServe())
}

func (cfg *apiConfig) middlewareMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(int32(1))
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, map[string]string{"error": msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		data, _ = json.Marshal(map[string]string{"marshaling error": err.Error()})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(data)
}
