package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func main() {
	mux := http.NewServeMux()

	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	apiCfg := apiConfig{}

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

		data, err := json.Marshal(map[string]bool{"valid": true})
		if err != nil {
			log.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})	

	log.Fatal(server.ListenAndServe())
}

func (cfg *apiConfig) middlewareMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(int32(1))
		next.ServeHTTP(w, r)
	})
}
