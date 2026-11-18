package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"campuslostfound/backend"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func publicHTTPSOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && origin == "https://"+parsed.Host
}

func run() error {
	initialize := flag.Bool("init-test-accounts", false, "Read four test passwords from stdin: student, reception, affairs, admin")
	backup := flag.String("backup", "", "Write a consistent SQLite snapshot to a new file")
	flag.Parse()
	if *initialize && *backup != "" {
		return errors.New("choose one operation at a time")
	}

	dataDirectory := os.Getenv("CAMPUS_DATA_DIR")
	if dataDirectory == "" || !filepath.IsAbs(dataDirectory) {
		return errors.New("CAMPUS_DATA_DIR must be an absolute path on persistent storage")
	}
	databaseFile := filepath.Join(dataDirectory, "campus.db")
	if *backup != "" {
		return backend.BackupFile(databaseFile, *backup)
	}
	db, err := backend.OpenStore(databaseFile)
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

	origin := os.Getenv("APP_ORIGIN")
	if !publicHTTPSOrigin(origin) {
		return errors.New("APP_ORIGIN must be the public HTTPS origin without a path or trailing slash")
	}
	webOrigin := os.Getenv("CAMPUS_WEB_ORIGIN")
	if webOrigin != "" && !publicHTTPSOrigin(webOrigin) {
		return errors.New("CAMPUS_WEB_ORIGIN must be the website's HTTPS origin without a path or trailing slash")
	}
	if err := backend.LoadHoldingPeriod(db); err != nil {
		return err
	}
	api := backend.NewAPI(db)
	api.SecureCookies = true
	api.AllowedOrigin = origin
	api.AllowedWebOrigin = webOrigin
	api.TrustLocalProxy = true
	api.EnableSignup = os.Getenv("CAMPUS_SIGNUP") == "true"
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var ready bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE role='admin')").Scan(&ready); err != nil || !ready {
			http.Error(w, "database or administrator account unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// The HTTPS reverse proxy is the only public entry point.
	server := &http.Server{
		Addr:              "127.0.0.1:3000",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32_768,
	}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("Campus API listening on %s", server.Addr)
	select {
	case err := <-done:
		return err
	case <-signals.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}
