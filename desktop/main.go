package main

import (
	"campuslostfound/backend"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:ui
var bundledUI embed.FS

// Shared-server builds set this at link time. Source builds default to a local preview.
var packagedServerURL string

func main() {
	serve := flag.Bool("serve", false, "Run the shared service without a desktop window")
	address := flag.String("listen", "127.0.0.1:3000", "Shared service address")
	flag.Parse()
	root, _ := fs.Sub(bundledUI, "ui")
	var api http.Handler
	remote, configured := os.LookupEnv("CAMPUS_SERVER_URL")
	if !configured {
		remote = packagedServerURL
	}
	if remote != "" && !*serve {
		target, err := url.Parse(remote)
		if err != nil || target.Host == "" {
			log.Fatal("CAMPUS_SERVER_URL must be a valid service URL")
		}
		local := target.Hostname() == "127.0.0.1" || target.Hostname() == "localhost" || target.Hostname() == "::1"
		if target.Scheme != "https" && !(local && target.Scheme == "http") {
			log.Fatal("Remote services require HTTPS")
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		original := proxy.Director
		proxy.Director = func(r *http.Request) {
			original(r)
			r.Header.Set("Origin", target.Scheme+"://"+target.Host)
			r.Host = target.Host
		}
		api = proxy
	} else {
		dataDir := os.Getenv("CAMPUS_DATA_DIR")
		if dataDir == "" {
			base, err := os.UserConfigDir()
			if err != nil {
				log.Fatal(err)
			}
			dataDir = filepath.Join(base, "CampusLostFound")
		}
		db, err := backend.OpenStore(filepath.Join(dataDir, "campus.db"))
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		localAPI := backend.NewAPI(db)
		localAPI.AllowSetup = !*serve || os.Getenv("CAMPUS_ALLOW_SETUP") == "true"
		localAPI.SecureCookies = os.Getenv("SECURE_COOKIES") == "true"
		localAPI.AllowedOrigin = os.Getenv("APP_ORIGIN")
		api = localAPI
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/", http.FileServer(http.FS(root)))
	if *serve {
		server := &http.Server{Addr: *address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
		log.Printf("Campus service: http://%s", *address)
		log.Fatal(server.ListenAndServe())
	}
	err := wails.Run(&options.App{
		Title: "Campus Lost & Found", Width: 1280, Height: 850, MinWidth: 380, MinHeight: 600,
		BackgroundColour: &options.RGBA{R: 247, G: 248, B: 246, A: 255},
		AssetServer:      &assetserver.Options{Assets: root, Handler: mux},
	})
	if err != nil {
		log.Fatal(err)
	}
}
