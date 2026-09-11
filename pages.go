package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
)

// Section is one analytics page in the keyboard menu.
type Section struct {
	Key   string // access key revealed when alt is held
	Path  string
	Title string
	Blurb string
}

// sections is the menu. Order here is the order shown in the overlay.
var sections = []Section{
	{"T", "/", "typing test", "back to the test"},
	{"O", "/overview", "overview", "what the data says about you"},
	{"K", "/keys", "keys", "per key, finger, hand and row"},
	{"E", "/errors", "errors", "what you miss and how you recover"},
	{"R", "/rhythm", "rhythm", "timing, pauses and consistency"},
	{"W", "/words", "words", "word length, hard words, position"},
	{"H", "/history", "history", "progress, time of day, streaks"},
	{"X", "/text", "text", "what the test is made of"},
	{"S", "/setup", "setup", "where you are and what you type on"},
}

// Page is the data every analytics template needs.
type Page struct {
	Sections []Section
	Active   string
	Title    string
	Runs     int
	Empty    bool
	Data     any
}

// render fills in the chrome common to every page.
func render(w http.ResponseWriter, r *http.Request, tmpl, active, title string, data any) {
	// Own runs only, matching what the tables below actually aggregate. A
	// header claiming 30 runs over tables built from 27 invites exactly the
	// mistrust the evidence gates exist to prevent.
	runs := 0
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM v_own_runs`).Scan(&runs); err != nil {
		log.Printf("count runs: %v", err)
	}
	p := Page{
		Sections: sections,
		Active:   active,
		Title:    title,
		Runs:     runs,
		Empty:    runs == 0,
		Data:     data,
	}
	w.Header().Set("Cache-Control", "no-store")
	if err := pages.ExecuteTemplate(w, tmpl, p); err != nil {
		log.Printf("render %s: %v", tmpl, err)
	}
}

/* ------------------------------------------------------------------ rows */

// Row is a generic labelled measurement, which is most of what these pages
// show. Using one shape keeps the templates small.
type Row struct {
	Label    string
	Count    int
	Accuracy float64
	Millis   int
	Extra    string
}

// Table is a titled group of rows with a unit for its bars.
type Table struct {
	Title string
	Note  string
	Unit  string
	Rows  []Row
	// Bars scale against the largest Millis when true, otherwise Count.
	ByTime bool
}

/* ------------------------------------------------------------------ pages */

// handleOverview is the interpreted view: plain statements about the typist,
// with the same evidence gates the results screen used to apply. It is where
// the lifetime findings moved to when the post-run screen was trimmed back to
// describing a single run.
func handleOverview(w http.ResponseWriter, r *http.Request) {
	render(w, r, "overview.html", "/overview", "overview", nil)
}

// handleText is where the shape of the test itself is chosen: which sources
// make up the text, and what decorates them. Separate from /setup because that
// page labels runs, while this one changes what you are about to type -- and
// because three groups on one page did not fit a screen.
func handleText(w http.ResponseWriter, r *http.Request) {
	render(w, r, "text.html", "/text", "text", nil)
}

// handleSetup is where the two context tags are chosen by hand. Detection
// covers the cases a browser can see; this covers the ones it cannot -- a
// place that is neither work nor home, and a guest at the keyboard.
func handleSetup(w http.ResponseWriter, r *http.Request) {
	render(w, r, "setup.html", "/setup", "setup", nil)
}

func handleKeys(w http.ResponseWriter, r *http.Request) {
	tables := []Table{}

	tables = append(tables, s2table(
		"per finger", "accuracy and pace by the finger that should have moved", "ms", true,
		`SELECT finger || ' (' || hand || ')', attempts, accuracy, mean_flight_ms
		 FROM v_finger ORDER BY mean_flight_ms DESC`))

	tables = append(tables, s2table(
		"per hand", "", "ms", true,
		`SELECT hand, attempts, accuracy, mean_flight_ms FROM v_hand ORDER BY hand`))

	tables = append(tables, s2table(
		"per row", "reaching away from home costs time", "ms", true,
		`SELECT row, attempts, accuracy, mean_flight_ms
		 FROM v_row ORDER BY mean_flight_ms DESC`))

	tables = append(tables, s2table(
		"slowest keys", "at least 10 attempts", "ms", true,
		`SELECT key, attempts, accuracy, mean_flight_ms
		 FROM v_key_skill WHERE attempts >= 10 AND key <> ' '
		 ORDER BY mean_flight_ms DESC LIMIT 15`))

	tables = append(tables, s2table(
		"least accurate keys", "at least 10 attempts", "%", false,
		`SELECT key, attempts, accuracy, mean_flight_ms
		 FROM v_key_skill WHERE attempts >= 10 AND key <> ' '
		 ORDER BY accuracy ASC LIMIT 15`))

	tables = append(tables, s2table(
		"how long you hold keys", "dwell time, distinct from the gap between keys", "ms", true,
		`SELECT key, presses, 0, mean_hold_ms FROM v_dwell
		 WHERE presses >= 10 AND key <> ' ' ORDER BY mean_hold_ms DESC LIMIT 12`))

	render(w, r, "analytics.html", "/keys", "keys", tables)
}

func handleErrors(w http.ResponseWriter, r *http.Request) {
	tables := []Table{}

	tables = append(tables, s2table(
		"what you hit instead", "the confusion matrix", "", false,
		`SELECT 'meant ' || expected || ', hit ' || typed, times, 0, 0
		 FROM v_confusions ORDER BY times DESC LIMIT 20`))

	tables = append(tables, s2table(
		"after a mistake", "keys following an error, against your clean pace", "ms", true,
		`SELECT CASE WHEN since_error = 0 THEN 'the mistake'
		              ELSE '+' || since_error || ' keys' END,
		        keys, accuracy, mean_flight_ms
		 FROM v_error_recovery ORDER BY since_error`))

	tables = append(tables, s2table(
		"do errors cluster", "error rate at each distance after a mistake", "%", false,
		`SELECT '+' || since_error || ' keys', keys, error_rate, 0
		 FROM v_error_clusters ORDER BY since_error`))

	tables = append(tables, s2table(
		"where in the word", "first letters fail differently from interiors", "%", false,
		`SELECT place, attempts, error_rate, mean_flight_ms FROM v_error_position`))

	tables = append(tables, s2table(
		"hardest words", "seen at least 3 times", "%", false,
		`SELECT word, seen, pct_clean, 0 FROM v_hard_words
		 ORDER BY pct_clean ASC, seen DESC LIMIT 20`))

	render(w, r, "analytics.html", "/errors", "errors", tables)
}

func handleRhythm(w http.ResponseWriter, r *http.Request) {
	tables := []Table{}

	tables = append(tables, s2table(
		"where the time goes", "every gap between keys, bucketed", "", false,
		`SELECT band, times, accuracy, 0 FROM v_pauses
		 ORDER BY CASE band
		   WHEN 'flow (<200ms)' THEN 1 WHEN 'steady (200-400ms)' THEN 2
		   WHEN 'thinking (400-800ms)' THEN 3 WHEN 'stalled (0.8-2s)' THEN 4
		   ELSE 5 END`))

	tables = append(tables, s2table(
		"hand transitions", "alternating hands is fastest, same finger slowest", "ms", true,
		`SELECT pattern, times, accuracy, mean_flight_ms
		 FROM v_transitions WHERE pattern <> 'unknown' ORDER BY mean_flight_ms DESC`))

	tables = append(tables, s2table(
		"row jumps", "cost of moving between rows", "ms", true,
		`SELECT move, times, accuracy, mean_flight_ms FROM v_row_jumps
		 WHERE times >= 20 ORDER BY mean_flight_ms DESC LIMIT 15`))

	tables = append(tables, s2table(
		"consistency per run", "spread of your keystroke timing; lower is steadier", "ms", true,
		`SELECT 'run ' || run_id, keys, 0, sd_ms FROM v_rhythm
		 ORDER BY run_id DESC LIMIT 15`))

	tables = append(tables, s2table(
		"slowest letter pairs", "at least 10 occurrences", "ms", true,
		`SELECT bigram, times, accuracy, mean_flight_ms FROM v_bigrams
		 WHERE times >= 10 ORDER BY mean_flight_ms DESC LIMIT 15`))

	tables = append(tables, s2table(
		"fastest letter pairs", "at least 10 occurrences", "ms", true,
		`SELECT bigram, times, accuracy, mean_flight_ms FROM v_bigrams
		 WHERE times >= 10 ORDER BY mean_flight_ms ASC LIMIT 15`))

	tables = append(tables, s2table(
		"slowest three-key runs", "where motor patterns break down", "ms", true,
		`SELECT trigram, times, accuracy, mean_span_ms FROM v_trigrams
		 ORDER BY mean_span_ms DESC LIMIT 15`))

	render(w, r, "analytics.html", "/rhythm", "rhythm", tables)
}

func handleWords(w http.ResponseWriter, r *http.Request) {
	tables := []Table{}

	tables = append(tables, s2table(
		"by word length", "per-key pace for words of each length", "ms", true,
		`SELECT length || ' letters', words, pct_clean, mean_flight_ms
		 FROM v_word_length ORDER BY length`))

	tables = append(tables, s2table(
		"where you drift", "position in the test, across all runs", "ms", true,
		`SELECT 'word ' || (word_idx + 1), keys, accuracy, mean_flight_ms
		 FROM v_focus_by_word ORDER BY mean_flight_ms DESC LIMIT 15`))

	tables = append(tables, s2table(
		"pace through the test", "by elapsed seconds", "ms", true,
		`SELECT second_bucket || 's', keys, accuracy, mean_flight_ms
		 FROM v_focus_by_time ORDER BY second_bucket`))

	tables = append(tables, s2table(
		"cleanest words", "seen at least 3 times", "%", false,
		`SELECT word, seen, pct_clean, 0 FROM v_hard_words
		 ORDER BY pct_clean DESC, seen DESC LIMIT 20`))

	render(w, r, "analytics.html", "/words", "words", tables)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	tables := []Table{}

	// started_at is UTC; tz_offset is JS getTimezoneOffset(), i.e. minutes WEST
	// of UTC, so it is SUBTRACTED to reach the typist's local wall clock. The
	// timestamp is formatted whole rather than rebuilt from local_hour, which
	// carries no minutes.
	tables = append(tables, s2table(
		"recent runs", "newest first", "wpm", true,
		`SELECT STRFTIME('%Y-%m-%d %H:%M',
		          (started_at - COALESCE(tz_offset,0)*60000)/1000, 'unixepoch'),
		        keystrokes, accuracy, wpm
		 FROM runs ORDER BY started_at DESC LIMIT 20`))

	tables = append(tables, s2table(
		"by day", "", "wpm", true,
		`SELECT day, runs, mean_accuracy, mean_wpm FROM v_days
		 ORDER BY day DESC LIMIT 20`))

	tables = append(tables, s2table(
		"by hour of day", "your local time", "wpm", true,
		`SELECT printf('%02d:00', local_hour), runs, mean_accuracy, mean_wpm
		 FROM v_by_hour ORDER BY local_hour`))

	tables = append(tables, s2table(
		"by day of week", "", "wpm", true,
		`SELECT CASE local_dow WHEN 0 THEN 'Sunday' WHEN 1 THEN 'Monday'
		         WHEN 2 THEN 'Tuesday' WHEN 3 THEN 'Wednesday'
		         WHEN 4 THEN 'Thursday' WHEN 5 THEN 'Friday'
		         ELSE 'Saturday' END, runs, mean_accuracy, mean_wpm
		 FROM v_by_dow ORDER BY local_dow`))

	render(w, r, "analytics.html", "/history", "history", tables)
}

/* ---------------------------------------------------------------- helpers */

// s2table runs a four-column query (label, count, accuracy, millis) into a
// Table. Every analytics query on these pages has that shape, so one helper
// covers all of them and the page handlers stay readable as a list of
// questions.
func s2table(title, note, unit string, byTime bool, query string) Table {
	t := Table{Title: title, Note: note, Unit: unit, ByTime: byTime, Rows: []Row{}}

	rows, err := store.db.Query(query)
	if err != nil {
		log.Printf("%s: %v", title, err)
		return t
	}
	defer rows.Close()

	for rows.Next() {
		var r Row
		var label sql.NullString
		var count sql.NullInt64
		var acc sql.NullFloat64
		var ms sql.NullInt64
		if err := rows.Scan(&label, &count, &acc, &ms); err != nil {
			log.Printf("%s scan: %v", title, err)
			return t
		}
		r.Label = label.String
		r.Count = int(count.Int64)
		r.Accuracy = acc.Float64
		r.Millis = int(ms.Int64)
		t.Rows = append(t.Rows, r)
	}
	if err := rows.Err(); err != nil {
		log.Printf("%s rows: %v", title, err)
	}
	return t
}

// Bar is the width percentage for a row.
//
// Counts are drawn from zero, because zero is meaningful for them. Times are
// not: every keystroke takes *some* time, so a 133ms-to-179ms spread drawn
// from zero renders as five near-identical full bars and hides the very
// difference the table exists to show. Time bars therefore span the range
// present, with a floor so the fastest row stays visible rather than vanishing.
func (t Table) Bar(r Row) string {
	if !t.ByTime {
		max := 0
		for _, row := range t.Rows {
			if row.Count > max {
				max = row.Count
			}
		}
		if max == 0 {
			return "0"
		}
		return fmt.Sprintf("%.1f", float64(r.Count)/float64(max)*100)
	}

	min, max := 0, 0
	for i, row := range t.Rows {
		if i == 0 || row.Millis < min {
			min = row.Millis
		}
		if row.Millis > max {
			max = row.Millis
		}
	}
	if max == min {
		return "100"
	}
	const floor = 12.0 // percent, so the smallest bar is still a bar
	span := float64(r.Millis-min) / float64(max-min)
	return fmt.Sprintf("%.1f", floor+span*(100-floor))
}

// Value is the number shown at the end of a row.
func (t Table) Value(r Row) string {
	if t.ByTime {
		return fmt.Sprintf("%d%s", r.Millis, t.Unit)
	}
	if t.Unit == "%" {
		return fmt.Sprintf("%.1f%%", r.Accuracy)
	}
	return fmt.Sprintf("%d", r.Count)
}
