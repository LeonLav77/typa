package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store persists runs. It writes raw capture only: no metric is computed on
// the way in, so every future question is asked of the same untouched data.
type Store struct {
	db *sql.DB
}

func openStore(path string) (*Store, error) {
	// _time_format and busy timeout keep concurrent writes from erroring out
	// under the WAL mode the schema sets.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Save writes one run and its full event stream in a single transaction, so a
// run is either wholly present or wholly absent — a half-written stream would
// silently corrupt every aggregate computed over it.
func (s *Store) Save(r *Run) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()

	if r.Typist != "" {
		if _, err := tx.Exec(
			`INSERT INTO typists (id, first_seen, last_seen) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET last_seen = excluded.last_seen`,
			r.Typist, now, now,
		); err != nil {
			return 0, fmt.Errorf("typist: %w", err)
		}
	}

	local := r.localTime()
	res, err := tx.Exec(`
		INSERT INTO runs (
			test_id, typist_id, schema_v, started_at, ended_at, duration_ms,
			local_hour, local_dow, tz, tz_offset,
			wpm, raw_wpm, accuracy, words, words_right,
			chars_typed, chars_correct, keystrokes,
			mode, location, location_src, device, device_src,
			layout, ua, platform, viewport_w, viewport_h, cores, memory,
			received_at
		) VALUES (?,?,?,?,?,?, ?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?,?,?,?, ?,?,?,?,?,?,?, ?)`,
		r.ID, nullStr(r.Typist), r.V, r.StartedAt, r.EndedAt, r.EndedAt-r.StartedAt,
		local.Hour(), int(local.Weekday()), r.Env.TZ, r.Env.TZOffset,
		r.Stats.WPM, r.Stats.Raw, r.Stats.Accuracy, r.Stats.Words, r.Stats.WordsRight,
		r.Stats.TypedChars, r.Stats.Correct, r.Stats.Keystrokes,
		nullStr(r.Mode),
		nullStr(r.Location), nullStr(r.LocationSrc), nullStr(r.Device), nullStr(r.DeviceSrc),
		nullStr(r.Layout), r.Env.UA, r.Env.Platform, r.Env.ViewportW, r.Env.ViewportH,
		r.Env.Cores, r.Env.Memory,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("run: %w", err)
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := insertWords(tx, runID, r); err != nil {
		return 0, err
	}
	if err := insertEvents(tx, runID, r); err != nil {
		return 0, err
	}
	return runID, tx.Commit()
}

func insertWords(tx *sql.Tx, runID int64, r *Run) error {
	stmt, err := tx.Prepare(`
		INSERT INTO run_words (run_id, idx, word, typed, correct,
			entered_ms, first_ms, last_ms, left_ms)
		VALUES (?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, w := range r.Words {
		var typed string
		if i < len(r.Typed) {
			typed = r.Typed[i]
		}
		var wt WordTime
		if i < len(r.WordTimes) {
			wt = r.WordTimes[i]
		}
		correct := 0
		if typed == w {
			correct = 1
		}
		if _, err := stmt.Exec(runID, i, w, typed, correct,
			wt.Entered, wt.First, wt.Last, wt.Left); err != nil {
			return fmt.Errorf("word %d: %w", i, err)
		}
	}
	return nil
}

func insertEvents(tx *sql.Tx, runID int64, r *Run) error {
	stmt, err := tx.Prepare(`
		INSERT INTO keystrokes (run_id, seq, kind, t_ms, key, code, expected, ok,
			word_idx, char_pos, mods, repeat, hold_ms, gap_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	prev := 0
	for i, e := range r.Events {
		gap := e.T - prev
		prev = e.T
		if _, err := stmt.Exec(runID, i, e.Kind, e.T, nullStr(e.Key), nullStr(e.Code),
			e.Expected, e.OK, e.Word, e.Pos, e.Mods, boolInt(e.Repeat), e.Hold, gap,
		); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// migrate adds columns that were introduced after a database was first
// created. SQLite applies `CREATE TABLE IF NOT EXISTS` as a no-op on an
// existing table, so new columns never appear from schema.sql alone — an
// existing database would keep working and silently lack them.
//
// Each ALTER is attempted and its "duplicate column" error ignored, which is
// idempotent without needing a version counter to be kept in sync by hand.
func (s *Store) migrate() error {
	alters := []string{
		`ALTER TABLE runs ADD COLUMN mode TEXT`,
		`ALTER TABLE runs ADD COLUMN location TEXT`,
		`ALTER TABLE runs ADD COLUMN location_src TEXT`,
		`ALTER TABLE runs ADD COLUMN device TEXT`,
		`ALTER TABLE runs ADD COLUMN device_src TEXT`,
	}
	for _, q := range alters {
		if _, err := s.db.Exec(q); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}
