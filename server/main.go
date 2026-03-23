package main

import (
	"log"
	"net/http"
	"os"
	"rbf-api/handler"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.NewMux(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("rbf-api listening on port %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
