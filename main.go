package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var pages = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// store is the run database. Nil only if the server was started without one.
var store *Store

// env returns the environment variable if set, else def. Flags stay the
// documented interface; env vars exist so a container can be configured
// without rewriting its command line.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", env("ADDR", ":8080"), "address to listen on")
	dbPath := flag.String("db", env("DB_PATH", "typing.db"), "path to the SQLite database")
	flag.Parse()

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	store, err = openStore(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err := store.migrate(); err != nil {
		log.Fatal(err)
	}
	if err := store.seedKeymap(); err != nil {
		log.Fatal(err)
	}
	if err := store.applyViews(); err != nil {
		log.Fatal(err)
	}
	log.Printf("recording runs to %s", *dbPath)

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("GET /api/test", handleNewTest)
	mux.HandleFunc("POST /api/results", handleResults)
	mux.HandleFunc("GET /api/insights", handleInsights)
	mux.HandleFunc("GET /overview", handleOverview)
	mux.HandleFunc("GET /setup", handleSetup)
	mux.HandleFunc("GET /text", handleText)
	mux.HandleFunc("GET /keys", handleKeys)
	mux.HandleFunc("GET /errors", handleErrors)
	mux.HandleFunc("GET /rhythm", handleRhythm)
	mux.HandleFunc("GET /words", handleWords)
	mux.HandleFunc("GET /modes", handleModes)
	mux.HandleFunc("GET /history", handleHistory)

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
		// Behind Traefik, but a slow or stuck client should not pin a
		// goroutine and a connection open forever regardless of what is in
		// front of us.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// `docker compose up -d` on a new image stops the old container with
	// SIGTERM. Draining in-flight requests matters here because a run POST
	// writes a whole event stream in one transaction — killed mid-write it
	// rolls back, and the typist loses the run they just finished.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		log.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("typing listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// handleHealth reports whether the process can still serve. It pings the
// database rather than returning a bare 200: a container whose disk or volume
// has gone away still accepts connections, and an unconditional "ok" would
// keep it in rotation.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := store.Ping(r.Context()); err != nil {
		log.Printf("health: %v", err)
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}` + "\n"))
}

// indexData carries both the dealt test and the menu, since the typing page
// renders the same navigation chrome as the analytics pages.
type indexData struct {
	Test
	Sections []Section
	Active   string
}

// handleIndex serves the page with a test already dealt into the markup, so
// the first paint needs no round trip.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// The mode rides on the query string so a reload keeps the text you chose;
	// with no query this is the plain-words test it has always been.
	m := modeFromQuery(r.URL.Query())
	data := indexData{Test: newTestMode(m), Sections: sections, Active: "/"}
	if err := pages.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("render index: %v", err)
	}
}

// handleNewTest deals a replacement test for esc / enter, so a new run never
// costs a page load.
func handleNewTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(newTestMode(modeFromQuery(r.URL.Query()))); err != nil {
		log.Printf("encode test: %v", err)
	}
}

// maxResultBytes bounds one submitted run. A 50-word run with full keydown,
// keyup and focus capture is well under 200KB; this leaves generous headroom
// while refusing anything pathological.
const maxResultBytes = 4 << 20 // 4MB

// handleResults stores one finished run verbatim. It validates only enough to
// keep the database honest — it deliberately computes nothing, because every
// derived metric is a query over the stored stream, not a column written here.
func handleResults(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxResultBytes))
	var run Run
	if err := dec.Decode(&run); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if run.StartedAt <= 0 || run.EndedAt < run.StartedAt {
		http.Error(w, "bad timestamps", http.StatusBadRequest)
		return
	}
	if len(run.Words) == 0 || len(run.Events) == 0 {
		http.Error(w, "empty run", http.StatusBadRequest)
		return
	}
	run.normalise()

	id, err := store.Save(&run)
	if err != nil {
		log.Printf("save run: %v", err)
		http.Error(w, "could not store run", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"run": id})
}

// handleInsights returns everything the results screen draws. Each number is
// computed at query time from the raw stream, so the endpoint's shape can grow
// without any change to what is being collected.
func handleInsights(w http.ResponseWriter, r *http.Request) {
	// `run` scopes the "this run" half; without it only lifetime is returned.
	runID, _ := strconv.ParseInt(r.URL.Query().Get("run"), 10, 64)
	ins, err := store.Insights(r.URL.Query().Get("typist"), runID, 30)
	if err != nil {
		log.Printf("insights: %v", err)
		http.Error(w, "could not read insights", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(ins); err != nil {
		log.Printf("encode insights: %v", err)
	}
}
