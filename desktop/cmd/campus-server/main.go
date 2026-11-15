package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"campuslostfound/backend"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	initialize := flag.Bool("init-test-accounts", false, "Read four test passwords from stdin: student, reception, affairs, admin")
	flag.Parse()

	dataDirectory := os.Getenv("CAMPUS_DATA_DIR")
	if dataDirectory == "" || !filepath.IsAbs(dataDirectory) {
		return errors.New("CAMPUS_DATA_DIR must be an absolute path on persistent storage")
	}
	db, err := backend.OpenStore(filepath.Join(dataDirectory, "campus.db"))
	if err != nil {
		return fmt.Errorf("open campus database: %w", err)
	}
	defer db.Close()

	if *initialize {
		var passwords [4]string
		reader := bufio.NewScanner(os.Stdin)
		used := make(map[string]bool)
		for index := range passwords {
			if !reader.Scan() {
				return errors.New("expected four distinct test passwords on stdin")
			}
			password := reader.Text()
			if len(password) < 12 || len(password) > 200 || used[password] {
				return errors.New("use four distinct test passwords of 12–200 bytes")
			}
			used[password] = true
			passwords[index] = password
		}
		if err := backend.SeedTestAccounts(db, passwords); err != nil {
			return fmt.Errorf("initialize test accounts: %w", err)
		}
		log.Print("Staging test accounts initialized.")
		return nil
	}

	api := backend.NewAPI(db)
	api.SecureCookies = true
	api.AllowedOrigin = os.Getenv("APP_ORIGIN")
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	// The HTTPS reverse proxy is the only public entry point.
	server := &http.Server{
		Addr:              "127.0.0.1:3000",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("Campus API listening on %s", server.Addr)
	return server.ListenAndServe()
}
