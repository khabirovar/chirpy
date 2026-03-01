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
	"github.com/khabirovar/chirpy/internal/database"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
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
		db:       database.New(db),
		platform: os.Getenv("PLATFORM"),
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
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Forbidden"))
			return
		}

		err := apiCfg.db.DeleteUsers(r.Context())
		if err != nil {
			log.Fatal(err)
		}

		apiCfg.fileserverHits.Swap(int32(0))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		msg := fmt.Sprintf(metricsPage, apiCfg.fileserverHits.Load())
		w.Write([]byte(msg))
	})

	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type RespBody struct {
			Body string `json:"body"`
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		body := RespBody{}
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			data, err := json.Marshal(map[string]string{"error": "Something went wrong"})
			if err != nil {
				log.Fatal(err)
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(data)
			return
		}

		if len(body.Body) > 140 {
			data, err := json.Marshal(map[string]string{"error": "Chirp is too long"})
			if err != nil {
				log.Fatal(err)
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write(data)
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

		data, err := json.Marshal(map[string]string{"cleaned_body": strings.Join(cleaned, " ")})
		if err != nil {
			log.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		type Params struct {
			Email string `json:"email"`
		}

		var params Params
		err := json.NewDecoder(r.Body).Decode(&params)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			data, err := json.Marshal(map[string]string{"error": err.Error()})
			if err != nil {
				log.Fatal(err)
			}
			w.Write(data)
		}

		type User struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Email     string `json:"email"`
		}

		var user User
		userFromDB, err := apiCfg.db.CreateUser(r.Context(), params.Email)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			data, err := json.Marshal(map[string]string{"error": err.Error()})
			if err != nil {
				log.Fatal(err)
			}
			w.Write(data)
		}
		user.ID = userFromDB.ID
		user.CreatedAt = userFromDB.CreatedAt
		user.UpdatedAt = userFromDB.UpdatedAt
		user.Email = userFromDB.Email

		userJson, err := json.Marshal(user)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			data, err := json.Marshal(map[string]string{"error": err.Error()})
			if err != nil {
				log.Fatal(err)
			}
			w.Write(data)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write(userJson)
	})

	log.Fatal(server.ListenAndServe())
}

func (cfg *apiConfig) middlewareMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(int32(1))
		next.ServeHTTP(w, r)
	})
}
