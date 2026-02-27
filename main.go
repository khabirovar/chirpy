package main

import (
	"net/http"
	"log"
)

func main() {
	mux := http.NewServeMux()
	
	server := http.Server{
		Handler: mux,
		Addr: ":8080",
	}
	
	mux.Handle("/", http.FileServer(http.Dir(".")))

	log.Fatal(server.ListenAndServe())
}
