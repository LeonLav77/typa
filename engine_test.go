package main

import (
	"database/sql"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPickWordsCount(t *testing.T) {
	got := pickWords(WordCount)
	if len(got) != WordCount {
		t.Fatalf("got %d words, want %d", len(got), WordCount)
	}
	for _, w := range got {
		if w == "" {
			t.Fatal("empty word dealt")
		}
	}
}

func TestWordListIsCleanAndUnique(t *testing.T) {
	ok := regexp.MustCompile(`^[a-z]+$`)
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		if !ok.MatchString(w) {
			t.Errorf("word %q is not lowercase letters only", w)
		}
		if seen[w] {
			t.Errorf("duplicate word %q", w)
		}
		seen[w] = true
	}
}

func TestChars(t *testing.T) {
	tt := Test{Words: []string{"go", "at"}}
	got := tt.Chars()
	want := [][]string{{"g", "o"}, {"a", "t"}}
	if len(got) != len(want) {
		t.Fatalf("got %d words, want %d", len(got), len(want))
	}
	for i := range want {
		if strings.Join(got[i], "") != strings.Join(want[i], "") {
			t.Errorf("word %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNewTestIDsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		id := newTest().ID
		if seen[id] {
			t.Fatalf("duplicate test id %q", id)
		}
		seen[id] = true
	}
}

func TestIndexRendersEveryWord(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if n := strings.Count(body, `class="word"`); n != WordCount {
		t.Errorf("rendered %d words, want %d", n, WordCount)
	}
	if !strings.Contains(body, "data-test-id=") {
		t.Error("no test id in markup")
	}
	if strings.Contains(body, "<no value>") {
		t.Error("template left an unresolved value")
	}
}

func TestAPITestReturnsFullTest(t *testing.T) {
	rec := httptest.NewRecorder()
	handleNewTest(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got Test
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Words) != WordCount {
		t.Errorf("got %d words, want %d", len(got.Words), WordCount)
	}
	if got.ID == "" {
		t.Error("empty test id")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	yes, no := true, false
	e, p := "a", 0
	run := &Run{
		V: 2, ID: "abc", Typist: "typist-1",
		StartedAt: 1700000000000, EndedAt: 1700000010000,
		Words: []string{"at"}, Typed: []string{"at"},
		WordTimes: []WordTime{{}},
		Events: []Event{
			{Kind: "down", T: 0, Key: "a", Code: "KeyA", Expected: &e, OK: &yes, Word: &p, Pos: &p},
			{Kind: "down", T: 90, Key: "z", Code: "KeyZ", Expected: &e, OK: &no, Word: &p, Pos: &p},
		},
		Env:   Env{TZ: "UTC", TZOffset: 0},
		Stats: Stats{WPM: 40, Accuracy: 50, Words: 1},
	}

	id, err := s.Save(run)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	var events, words int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM keystrokes WHERE run_id = ?`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Errorf("got %d keystrokes, want 2", events)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM run_words WHERE run_id = ?`, id).Scan(&words); err != nil {
		t.Fatal(err)
	}
	if words != 1 {
		t.Errorf("got %d words, want 1", words)
	}

	// A miss must persist as ok=0, distinct from a NULL (not applicable).
	var ok sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT ok FROM keystrokes WHERE run_id = ? AND seq = 1`, id).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	if !ok.Valid || ok.Int64 != 0 {
		t.Errorf("miss stored as %v, want 0", ok)
	}
}

func TestResultsRejectsGarbage(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"not json", `{`},
		{"empty run", `{"v":2,"startedAt":1,"endedAt":2,"words":[],"events":[]}`},
		{"bad times", `{"v":2,"startedAt":0,"endedAt":0,"words":["a"],"events":[{"k":"down"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/results", strings.NewReader(tc.body))
			handleResults(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400", rec.Code)
			}
		})
	}
}

// A run's own numbers must be scoped to that run, and the lifetime half must
// count every run. Conflating the two is the bug this test guards.
func TestInsightsSeparatesRunFromLifetime(t *testing.T) {
	dir := t.TempDir()
	st, err := openStore(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.applyViews(); err != nil {
		t.Fatal(err)
	}

	yes, no := true, false
	mk := func(word string, hit bool) int64 {
		e := string(word[0])
		ok := &yes
		if !hit {
			ok = &no
		}
		z := 0
		id, err := st.Save(&Run{
			V: 2, ID: "t", Typist: "u1",
			StartedAt: 1700000000000, EndedAt: 1700000005000,
			Words: []string{word}, Typed: []string{word},
			WordTimes: []WordTime{{}},
			Events: []Event{
				{Kind: "down", T: 0, Key: e, Code: "KeyA", Expected: &e, OK: ok, Word: &z, Pos: &z},
				{Kind: "down", T: 120, Key: e, Code: "KeyA", Expected: &e, OK: &yes, Word: &z, Pos: &z},
			},
			Stats: Stats{WPM: 40, Accuracy: 90, Words: 1, Keystrokes: 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	first := mk("at", false) // this run has an error
	mk("by", true)
	mk("do", true)

	ins, err := st.Insights("u1", first, 10)
	if err != nil {
		t.Fatal(err)
	}

	if ins.All == nil || ins.All.Runs != 3 {
		t.Errorf("lifetime runs = %v, want 3", ins.All)
	}
	if ins.This == nil {
		t.Fatal("this-run section missing")
	}
	if ins.This.ID != first {
		t.Errorf("this.ID = %d, want %d", ins.This.ID, first)
	}
	if ins.This.Errors != 1 {
		t.Errorf("this run errors = %d, want 1 (must not count other runs)", ins.This.Errors)
	}

	// Without a run id, only the lifetime half is populated.
	bare, err := st.Insights("u1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if bare.This != nil {
		t.Error("this-run section should be absent when no run is named")
	}
	if bare.All.Runs != 3 {
		t.Errorf("lifetime runs = %d, want 3", bare.All.Runs)
	}
}

// Every menu target must resolve, or alt+key navigates into a 404.
func TestMenuSectionsAreRoutable(t *testing.T) {
	dir := t.TempDir()
	st, err := openStore(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.seedKeymap(); err != nil {
		t.Fatal(err)
	}
	if err := st.applyViews(); err != nil {
		t.Fatal(err)
	}

	prev := store
	store = st
	defer func() { store = prev }()

	seen := map[string]bool{}
	for _, s := range sections {
		if s.Key == "" || len(s.Key) != 1 {
			t.Errorf("section %q has no single-letter access key", s.Title)
		}
		if seen[s.Key] {
			t.Errorf("duplicate access key %q", s.Key)
		}
		seen[s.Key] = true
	}

	// Every page in the menu must render against an empty database without
	// error -- the settings screens included, since they share the same chrome
	// and a template typo there fails exactly as quietly.
	handlers := map[string]http.HandlerFunc{
		"/keys":     handleKeys,
		"/errors":   handleErrors,
		"/rhythm":   handleRhythm,
		"/words":    handleWords,
		"/history":  handleHistory,
		"/overview": handleOverview,
		"/text":     handleText,
		"/setup":    handleSetup,
	}
	// Anything in the menu without a handler here is a page nothing checks.
	for _, sec := range sections {
		if sec.Path == "/" {
			continue // the typing page, covered by its own test
		}
		if _, ok := handlers[sec.Path]; !ok {
			t.Errorf("menu lists %s but no test renders it", sec.Path)
		}
	}

	for path, h := range handlers {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "data-menu") {
			t.Errorf("%s: menu markup missing", path)
		}
		if strings.Contains(body, "<no value>") {
			t.Errorf("%s: template left an unresolved value", path)
		}
	}
}

// Physical geometry must cover every key the word list can ask for, or
// per-finger analysis silently drops keystrokes.
func TestKeymapCoversAlphabet(t *testing.T) {
	for c := 'a'; c <= 'z'; c++ {
		code := "Key" + strings.ToUpper(string(c))
		if _, ok := keyGeometry(code); !ok {
			t.Errorf("no geometry for %s", code)
		}
	}
	if _, ok := keyGeometry("Space"); !ok {
		t.Error("no geometry for Space")
	}
}

// Local wall-clock handling. started_at is UTC and tz_offset follows the JS
// convention (minutes WEST of UTC, so east of Greenwich is negative). Getting
// the sign wrong files evening runs under the previous day.
func TestLocalTimeUsesTypistTimezone(t *testing.T) {
	// 2026-09-11 23:30 UTC, in a UTC+2 zone -> 2026-09-12 01:30 local.
	utc := time.Date(2026, 9, 11, 23, 30, 0, 0, time.UTC).UnixMilli()
	r := &Run{StartedAt: utc, Env: Env{TZOffset: -120}}

	local := r.localTime()
	if got, want := local.Hour(), 1; got != want {
		t.Errorf("local hour = %d, want %d", got, want)
	}
	if got, want := local.Day(), 12; got != want {
		t.Errorf("local day = %d, want %d (run must not fall on the UTC date)", got, want)
	}

	// And west of Greenwich, the offset is positive.
	west := &Run{StartedAt: utc, Env: Env{TZOffset: 300}} // UTC-5
	if got, want := west.localTime().Hour(), 18; got != want {
		t.Errorf("west local hour = %d, want %d", got, want)
	}
	if got, want := west.localTime().Day(), 11; got != want {
		t.Errorf("west local day = %d, want %d", got, want)
	}
}

// tagRun builds a minimal one-keystroke run with the given context tags, so
// the tests below differ only in the thing being tested.
func tagRun(id string, ok bool, location, device string, wpm int) *Run {
	e, p := "a", 0
	return &Run{
		V: 2, ID: id, Typist: "typist-1",
		StartedAt: 1700000000000, EndedAt: 1700000010000,
		Words: []string{"a"}, Typed: []string{"a"},
		WordTimes: []WordTime{{}},
		Events: []Event{
			{Kind: "down", T: 0, Key: "a", Code: "KeyA", Expected: &e, OK: &ok, Word: &p, Pos: &p},
		},
		Env:      Env{TZ: "UTC", TZOffset: 0},
		Location: location, LocationSrc: "manual",
		Device: device, DeviceSrc: "manual",
		Stats: Stats{WPM: wpm, Accuracy: 100, Words: 1},
	}
}

// A guest's run must be stored in full and yet reach no view that describes
// the typist. Storing it is what makes "let someone else try" safe; excluding
// it is what stops their attempt from becoming part of your history.
func TestGuestRunsAreStoredButExcluded(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.applyViews(); err != nil {
		t.Fatalf("views: %v", err)
	}

	// Two clean runs of mine, then a guest who missed every key.
	for i, r := range []*Run{
		tagRun("mine-1", true, "home", "moonlander", 90),
		tagRun("mine-2", true, "work", "moonlander", 90),
		tagRun("theirs", false, "home", "guest", 10),
	} {
		if _, err := s.Save(r); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	var stored, own int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 3 {
		t.Errorf("stored %d runs, want 3 — the guest run must not be discarded", stored)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM v_own_runs`).Scan(&own); err != nil {
		t.Fatal(err)
	}
	if own != 2 {
		t.Errorf("v_own_runs has %d, want 2", own)
	}

	// The guest missed every key. If their keystrokes leaked into the base
	// view, accuracy here would be 66.7 rather than 100.
	var attempts int
	var accuracy float64
	if err := s.db.QueryRow(
		`SELECT attempts, accuracy FROM v_key_skill WHERE key = 'a'`).Scan(&attempts, &accuracy); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || accuracy != 100 {
		t.Errorf("v_key_skill: %d attempts at %.1f%%, want 2 at 100%% — guest keystrokes leaked in",
			attempts, accuracy)
	}
}

// An unrecognised tag must not be stored. Storing it would silently create a
// fourth location that splits every aggregate, and the run itself is still
// worth keeping.
func TestUnknownTagsAreDropped(t *testing.T) {
	r := tagRun("x", true, "canteen", "typewriter", 50)
	r.LocationSrc = "psychic"
	r.DeviceSrc = "vibes"
	r.normalise()

	if r.Location != "" || r.LocationSrc != "" {
		t.Errorf("location %q/%q survived, want empty", r.Location, r.LocationSrc)
	}
	if r.Device != "" || r.DeviceSrc != "" {
		t.Errorf("device %q/%q survived, want empty", r.Device, r.DeviceSrc)
	}

	good := tagRun("y", true, "work", "moonlander", 50)
	good.normalise()
	if good.Location != "work" || good.Device != "moonlander" {
		t.Errorf("valid tags were dropped: %q / %q", good.Location, good.Device)
	}
}

// migrate must be safe to run against a database that already has the columns,
// since it runs on every start.
func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	for i := 0; i < 3; i++ {
		if err := s.migrate(); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	// The columns must actually be usable afterwards.
	if err := s.applyViews(); err != nil {
		t.Fatalf("views: %v", err)
	}
	if _, err := s.Save(tagRun("m", true, "work", "moonlander", 70)); err != nil {
		t.Fatalf("save after migrate: %v", err)
	}
	var loc string
	if err := s.db.QueryRow(`SELECT location FROM runs WHERE test_id = 'm'`).Scan(&loc); err != nil {
		t.Fatal(err)
	}
	if loc != "work" {
		t.Errorf("location %q, want work", loc)
	}
}

// Every mode must deal a full test of non-empty tokens. The generator loops
// until it has enough, so a bug there hangs or under-fills rather than
// erroring, and neither shows up without asserting the count.
func TestGenerateFillsEveryMode(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, m := range []Mode{
		{Words: true},
		{Numbers: true},
		{Words: true, Numbers: true},
		{Words: true, Caps: true},
		{Words: true, Punctuation: true},
		{Words: true, Caps: true, Punctuation: true},
		{Words: true, Numbers: true, Caps: true, Punctuation: true},
		{Numbers: true, Punctuation: true},
	} {
		got := generate(r, WordCount, m)
		if len(got) != WordCount {
			t.Errorf("%s: got %d tokens, want %d", m, len(got), WordCount)
		}
		for i, tok := range got {
			if tok == "" {
				t.Errorf("%s: token %d is empty", m, i)
			}
			if strings.ContainsAny(tok, " \t\n") {
				t.Errorf("%s: token %q contains whitespace, which would split it", m, tok)
			}
		}
	}
}

// A mode with no source cannot produce text. Rather than dealing a blank test
// it falls back to words, since a blank screen is never the useful outcome.
func TestEmptyModeFallsBackToWords(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	got := generate(r, 10, Mode{})
	if len(got) != 10 {
		t.Fatalf("got %d tokens, want 10", len(got))
	}
	for _, tok := range got {
		if !regexp.MustCompile(`^[a-z]+$`).MatchString(tok) {
			t.Errorf("token %q is not a plain word", tok)
		}
	}
}

// Numbers-only must contain no letters, which is the whole point of being able
// to switch words off.
func TestNumbersOnlyHasNoLetters(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, tok := range generate(r, 60, Mode{Numbers: true}) {
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(tok) {
			t.Errorf("token %q is not digits only", tok)
		}
	}
}

// Caps must produce at least one capital, and none at all when off. A flag
// that silently does nothing is worse than no flag.
func TestCapsActuallyCapitalises(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	hasUpper := func(toks []string) bool {
		for _, tok := range toks {
			if strings.ToLower(tok) != tok {
				return true
			}
		}
		return false
	}
	if !hasUpper(generate(r, WordCount, Mode{Words: true, Caps: true})) {
		t.Error("caps on: no capital in a full test")
	}
	if !hasUpper(generate(r, WordCount, Mode{Words: true, Caps: true, Punctuation: true})) {
		t.Error("caps + punctuation: no capital in a full test")
	}
	if hasUpper(generate(r, WordCount, Mode{Words: true})) {
		t.Error("caps off: found a capital")
	}
}

// Punctuation must close what it opens: an unbalanced bracket or quote is a
// token the typist cannot resolve from the text in front of them.
func TestPunctuationPairsAreBalanced(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	for run := 0; run < 40; run++ {
		for _, tok := range generate(r, WordCount, Mode{Words: true, Punctuation: true, Caps: true}) {
			counts := map[rune]int{}
			for _, c := range tok {
				counts[c]++
			}
			for close, open := range pairs {
				if counts[open] != counts[close] {
					t.Fatalf("token %q has unbalanced %c%c", tok, open, close)
				}
			}
			for _, q := range []rune{'"', '\''} {
				if counts[q]%2 != 0 {
					t.Fatalf("token %q has an odd number of %c", tok, q)
				}
			}
		}
	}
}

// A sentence-ending mark belongs outside a closing bracket or quote, not
// inside it: ('word'). rather than ('word.')
func TestSentenceMarksSitOutsideWrappers(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	bad := regexp.MustCompile(`[.?!,;:][)\]}"']`)
	for run := 0; run < 40; run++ {
		for _, tok := range generate(r, WordCount, Mode{Words: true, Punctuation: true}) {
			if bad.MatchString(tok) {
				t.Fatalf("token %q buries punctuation inside a wrapper", tok)
			}
		}
	}
}

// The mode tag is what analysis groups by, so it must be stable and ordered.
func TestModeTag(t *testing.T) {
	for _, tc := range []struct {
		m    Mode
		want string
	}{
		{Mode{Words: true}, "words"},
		{Mode{Numbers: true}, "numbers"},
		{Mode{Words: true, Numbers: true}, "words-numbers"},
		{Mode{Words: true, Caps: true, Punctuation: true}, "words-caps-punctuation"},
		{Mode{}, "words"},
	} {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("%#v tag = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// The query parser decides what a URL means, and an absent parameter must keep
// the old behaviour so existing links and the bare page are unchanged.
func TestModeFromQuery(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want Mode
	}{
		{"", Mode{Words: true}},
		{"words=true", Mode{Words: true}},
		{"words=false&numbers=true", Mode{Numbers: true}},
		{"numbers=true", Mode{Words: true, Numbers: true}},
		{"words=true&caps=true&punctuation=true", Mode{Words: true, Caps: true, Punctuation: true}},
		// No source at all cannot make text; fall back rather than deal blank.
		{"words=false&numbers=false", Mode{Words: true}},
		// Garbage keeps the default rather than failing the request.
		{"words=banana", Mode{Words: true}},
	} {
		q, err := url.ParseQuery(tc.q)
		if err != nil {
			t.Fatal(err)
		}
		if got := modeFromQuery(q); got != tc.want {
			t.Errorf("%q -> %#v, want %#v", tc.q, got, tc.want)
		}
	}
}

// With caps and punctuation on, a sentence must not open on a number: a digit
// cannot take a capital, so it reads as a missing one.
func TestSentencesDoNotOpenOnANumber(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	m := Mode{Words: true, Numbers: true, Caps: true, Punctuation: true}
	ends := regexp.MustCompile(`[.?!]["')\]}]?$`)

	for run := 0; run < 60; run++ {
		toks := generate(r, WordCount, m)
		for i, tok := range toks {
			opensSentence := i == 0 || ends.MatchString(toks[i-1])
			if !opensSentence {
				continue
			}
			// Skip a leading wrapper to reach the first real character.
			first := strings.TrimLeft(tok, `"'([{`)
			if first == "" {
				continue
			}
			if first[0] >= '0' && first[0] <= '9' {
				t.Fatalf("sentence opens on a number: %q (after %q)", tok, previous(toks, i))
			}
		}
	}
}

func previous(toks []string, i int) string {
	if i == 0 {
		return "(start)"
	}
	return toks[i-1]
}

// Each slider level must be measurably denser than the one below it. A slider
// whose stops feel the same is worse than no slider: it invites fiddling that
// changes nothing.
func TestSliderLevelsAreMonotonic(t *testing.T) {
	const runs, n = 300, 50

	rate := func(lv int, mk func(int) Mode, count func(string) bool) float64 {
		r := rand.New(rand.NewSource(int64(lv) * 99))
		hits, total := 0, 0
		for i := 0; i < runs; i++ {
			for _, tok := range generate(r, n, mk(lv)) {
				total++
				if count(tok) {
					hits++
				}
			}
		}
		return float64(hits) / float64(total)
	}

	isNum := func(s string) bool { return s != "" && s[0] >= '0' && s[0] <= '9' }
	isCap := func(s string) bool {
		s = strings.TrimLeft(s, `"'([{`)
		return s != "" && s[0] >= 'A' && s[0] <= 'Z'
	}
	hasMark := func(s string) bool { return strings.ContainsAny(s, `.?!,;:"'()[]{}-`) }

	for _, tc := range []struct {
		name  string
		mk    func(int) Mode
		count func(string) bool
	}{
		{"numbers", func(lv int) Mode { return Mode{Words: true, Numbers: true, NumberLevel: lv} }, isNum},
		{"capitals", func(lv int) Mode {
			return Mode{Words: true, Caps: true, Punctuation: true, CapLevel: lv}
		}, isCap},
		{"punctuation", func(lv int) Mode { return Mode{Words: true, Punctuation: true, PunctLevel: lv} }, hasMark},
	} {
		prev := -1.0
		for lv := 1; lv <= levels; lv++ {
			got := rate(lv, tc.mk, tc.count)
			if got <= prev {
				t.Errorf("%s: level %d (%.1f%%) is not denser than level %d (%.1f%%)",
					tc.name, lv, got*100, lv-1, prev*100)
			}
			prev = got
		}
	}
}

// A level rides with its flag in the tag, because two differently weighted
// runs are different exercises and pooling them would be wrong.
func TestModeTagCarriesLevels(t *testing.T) {
	for _, tc := range []struct {
		m    Mode
		want string
	}{
		// The default level is left off, so the common tag stays readable.
		{Mode{Words: true, Numbers: true, NumberLevel: defaultLevel}, "words-numbers"},
		{Mode{Words: true, Numbers: true}, "words-numbers"},
		{Mode{Words: true, Numbers: true, NumberLevel: 5}, "words-numbers5"},
		{Mode{Words: true, Caps: true, CapLevel: 1}, "words-caps1"},
		{Mode{Words: true, Punctuation: true, PunctLevel: 4}, "words-punctuation4"},
		// A level on a flag that is off must not appear.
		{Mode{Words: true, NumberLevel: 5}, "words"},
	} {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("%#v tag = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// An out-of-range or absent level must fall back to the default rather than
// indexing past the rate tables and panicking.
func TestLevelsClampSafely(t *testing.T) {
	r := rand.New(rand.NewSource(12))
	for _, lv := range []int{-5, 0, 1, 5, 6, 99} {
		m := Mode{Words: true, Numbers: true, Caps: true, Punctuation: true,
			NumberLevel: lv, CapLevel: lv, PunctLevel: lv}
		if got := generate(r, WordCount, m); len(got) != WordCount {
			t.Errorf("level %d: got %d tokens, want %d", lv, len(got), WordCount)
		}
	}
}
