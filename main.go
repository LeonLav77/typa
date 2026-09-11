package main

import (
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var pages = template.Must(template.ParseFS(templateFS, "templates/*.html"))

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("GET /api/test", handleNewTest)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("typing listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// handleIndex serves the page with a test already dealt into the markup, so
// the first paint needs no round trip.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := pages.ExecuteTemplate(w, "index.html", newTest()); err != nil {
		log.Printf("render index: %v", err)
	}
}

// handleNewTest deals a replacement test for esc / enter, so a new run never
// costs a page load.
func handleNewTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(newTest()); err != nil {
		log.Printf("encode test: %v", err)
	}
}
