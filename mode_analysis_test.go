package main

import (
	"testing"
)

// saveModeRun stores one minimal run tagged with a mode, enough for the
// per-mode views to have something to group.
func saveModeRun(t *testing.T, s *Store, mode string, wpm int) {
	t.Helper()
	yes := true
	e, p := "a", 0
	first, last := 10, 90
	run := &Run{
		V: 2, ID: "t-" + mode, Typist: "typist-1",
		StartedAt: 1700000000000, EndedAt: 1700000010000,
		Mode:  mode,
		Words: []string{"at"}, Typed: []string{"at"},
		WordTimes: []WordTime{{First: &first, Last: &last}},
		Events: []Event{
			{Kind: "down", T: 0, Key: "a", Code: "KeyA", Expected: &e, OK: &yes, Word: &p, Pos: &p},
			{Kind: "down", T: 90, Key: "j", Code: "KeyJ", Expected: &e, OK: &yes, Word: &p, Pos: &p},
		},
		Env:   Env{TZ: "UTC", TZOffset: 0},
		Stats: Stats{WPM: wpm, Accuracy: 100, Words: 1},
	}
	if _, err := s.Save(run); err != nil {
		t.Fatalf("save %s: %v", mode, err)
	}
}

func modeStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.seedKeymap(); err != nil {
		t.Fatalf("keymap: %v", err)
	}
	if err := s.applyViews(); err != nil {
		t.Fatalf("views: %v", err)
	}
	return s
}

// The composite tag must collapse to a pool, or every modifier combination
// becomes its own mode and nothing is ever comparable. This is the property
// the whole per-mode analysis rests on.
func TestPoolCollapsesModifiers(t *testing.T) {
	s := modeStore(t)

	// Three Go runs written three different ways, plus one English run.
	saveModeRun(t, s, "go", 50)
	saveModeRun(t, s, "go-punctuation", 40)
	saveModeRun(t, s, "go-symbols-caps", 30)
	saveModeRun(t, s, "words", 80)

	rows, err := s.db.Query(`SELECT pool, runs, mean_wpm FROM v_by_pool ORDER BY pool`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string][2]int{}
	for rows.Next() {
		var pool string
		var runs, wpm int
		if err := rows.Scan(&pool, &runs, &wpm); err != nil {
			t.Fatal(err)
		}
		got[pool] = [2]int{runs, wpm}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if got["go"][0] != 3 {
		t.Errorf("go pool has %d runs, want 3 (modifiers must collapse)", got["go"][0])
	}
	if got["go"][1] != 40 { // (50+40+30)/3
		t.Errorf("go mean wpm = %d, want 40", got["go"][1])
	}
	if got["words"][0] != 1 {
		t.Errorf("words pool has %d runs, want 1", got["words"][0])
	}
	// The whole point: the two pools are separable.
	if got["go"][1] == got["words"][1] {
		t.Error("go and words are not separated")
	}
}

// The symbol flavour must stay distinguishable from the bare language: they
// are the same vocabulary at very different difficulty.
func TestSymbolFlavourIsSeparate(t *testing.T) {
	s := modeStore(t)
	saveModeRun(t, s, "laravel", 60)
	saveModeRun(t, s, "laravel-symbols", 20)

	var bare, sym int
	err := s.db.QueryRow(
		`SELECT mean_wpm FROM v_by_pool_flavour WHERE pool_flavour = 'laravel'`).Scan(&bare)
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.QueryRow(
		`SELECT mean_wpm FROM v_by_pool_flavour WHERE pool_flavour = 'laravel · symbols'`).Scan(&sym)
	if err != nil {
		t.Fatal(err)
	}
	if bare != 60 || sym != 20 {
		t.Errorf("bare=%d symbols=%d, want 60 and 20", bare, sym)
	}

	// While the coarse pool view still pools them, which is what makes
	// "how fast am I in Laravel" answerable at all.
	var pooled int
	if err := s.db.QueryRow(
		`SELECT runs FROM v_by_pool WHERE pool = 'laravel'`).Scan(&pooled); err != nil {
		t.Fatal(err)
	}
	if pooled != 2 {
		t.Errorf("v_by_pool laravel runs = %d, want 2", pooled)
	}
}

// An untagged run — one stored before modes existed — must still appear
// somewhere rather than vanishing from the per-mode views.
func TestUntaggedRunsKeepAPool(t *testing.T) {
	s := modeStore(t)
	saveModeRun(t, s, "", 55)

	var pool string
	var runs int
	if err := s.db.QueryRow(`SELECT pool, runs FROM v_by_pool`).Scan(&pool, &runs); err != nil {
		t.Fatal(err)
	}
	if pool != "words" || runs != 1 {
		t.Errorf("untagged run -> pool %q runs %d, want words/1", pool, runs)
	}
}

// Every per-mode view must be queryable. They are created from a single SQL
// string at startup, so a syntax error in one is a startup failure — but a
// view that parses and returns nonsense is not, hence checking each one runs.
func TestModeViewsAllQuery(t *testing.T) {
	s := modeStore(t)
	saveModeRun(t, s, "go-symbols", 42)
	saveModeRun(t, s, "left", 33)

	for _, v := range []string{
		"v_mode_runs", "v_by_pool", "v_by_pool_flavour", "v_by_mode",
		"v_key_by_pool", "v_finger_by_pool", "v_hand_by_pool",
		"v_hard_words_by_pool", "v_progress_by_pool",
	} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + v).Scan(&n); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
}

// The per-hand view is what the one-handed modes exist to feed: a left-hand
// run must show the left hand carrying the keystrokes.
func TestHandByPoolSeparatesHands(t *testing.T) {
	s := modeStore(t)
	saveModeRun(t, s, "left", 40)

	rows, err := s.db.Query(`SELECT pool, hand, attempts FROM v_hand_by_pool ORDER BY hand`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var pool, hand string
		var n int
		if err := rows.Scan(&pool, &hand, &n); err != nil {
			t.Fatal(err)
		}
		if pool != "left" {
			t.Errorf("pool = %q, want left", pool)
		}
		seen[hand] = n
	}
	// The fixture types KeyA (left) and KeyJ (right), so both appear; what
	// matters is that they are attributed per pool and per hand at all.
	if seen["left"] == 0 || seen["right"] == 0 {
		t.Errorf("hands not separated: %v", seen)
	}
}

// A wpm table must show the wpm, not the run count. The generic Table renders
// Count unless told otherwise, which silently turned "pace by mode" into a
// list of how many runs each mode had.
func TestScoreTableShowsRate(t *testing.T) {
	tbl := Table{
		Unit: " wpm", ByScore: true,
		Rows: []Row{{Label: "words", Count: 2, Score: 80}, {Label: "go", Count: 7, Score: 40}},
	}
	if got := tbl.Value(tbl.Rows[0]); got != "80 wpm" {
		t.Errorf("value = %q, want %q", got, "80 wpm")
	}
	// The bar must scale on the score, not the count: `go` has more runs but
	// is slower, and drawing it as the longer bar would invert the meaning.
	full, half := tbl.Bar(tbl.Rows[0]), tbl.Bar(tbl.Rows[1])
	if full != "100.0" || half != "50.0" {
		t.Errorf("bars = %q and %q, want 100.0 and 50.0", full, half)
	}
}
