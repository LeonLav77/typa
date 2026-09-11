package main

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed analysis.sql
var analysisSQL string

// applyViews installs the analysis views. They are plain SELECTs over the raw
// tables, so they can be dropped and redefined freely — changing how an
// insight is computed never touches the stored data.
func (s *Store) applyViews() error {
	if _, err := s.db.Exec(analysisSQL); err != nil {
		return fmt.Errorf("apply analysis views: %w", err)
	}
	return nil
}

// Insights separates two questions that must not be conflated:
//
//	This  — what happened in the run just finished. Always meaningful, because
//	        it describes one concrete test rather than claiming a pattern.
//	All   — what you are like as a typist. Only meaningful once enough runs
//	        exist; the client refuses to draw conclusions before then.
//
// Reporting a single run's quirks as lifetime traits is the failure mode this
// split exists to prevent.
type Insights struct {
	This *RunDetail `json:"this"`
	All  *Lifetime  `json:"all"`
}

// RunDetail describes exactly one run.
type RunDetail struct {
	ID          int64          `json:"id"`
	WPM         int            `json:"wpm"`
	Accuracy    int            `json:"accuracy"`
	Seconds     float64        `json:"seconds"`
	Keystrokes  int            `json:"keystrokes"`
	Errors      int            `json:"errors"`
	Pace        []PaceBucket   `json:"pace"`
	ByWord      []WordBucket   `json:"byWord"`
	Misses      []Confusion    `json:"misses"`
	SlowKeys    []KeyStat      `json:"slowKeys"`
	Recovery    []RecoveryStep `json:"recovery"`
	CleanFlight int            `json:"cleanFlight"`
}

// Lifetime is the accumulated picture across runs.
type Lifetime struct {
	Runs        int            `json:"runs"`
	Keystrokes  int            `json:"keystrokes"`
	Recent      []RunSummary   `json:"recent"`
	Pace        []PaceBucket   `json:"pace"`
	ByWord      []WordBucket   `json:"byWord"`
	Keys        []KeyStat      `json:"keys"`
	Confusions  []Confusion    `json:"confusions"`
	Recovery    []RecoveryStep `json:"recovery"`
	CleanFlight int            `json:"cleanFlight"`
	Doubles     *DoubleStat    `json:"doubles"`
}

type RunSummary struct {
	ID        int64 `json:"id"`
	StartedAt int64 `json:"startedAt"`
	WPM       int   `json:"wpm"`
	Accuracy  int   `json:"accuracy"`
	Seconds   int   `json:"seconds"`
}

type PaceBucket struct {
	Second      int     `json:"second"`
	Keys        int     `json:"keys"`
	Accuracy    float64 `json:"accuracy"`
	MeanFlight  int     `json:"meanFlight"`
	Hesitations int     `json:"hesitations"`
}

type WordBucket struct {
	Word        int     `json:"word"`
	Keys        int     `json:"keys"`
	Accuracy    float64 `json:"accuracy"`
	MeanFlight  int     `json:"meanFlight"`
	Hesitations int     `json:"hesitations"`
}

type KeyStat struct {
	Key        string  `json:"key"`
	Attempts   int     `json:"attempts"`
	Accuracy   float64 `json:"accuracy"`
	MeanFlight int     `json:"meanFlight"`
}

type Confusion struct {
	Expected string `json:"expected"`
	Typed    string `json:"typed"`
	Code     string `json:"code"`
	Times    int    `json:"times"`
}

type RecoveryStep struct {
	Since      int     `json:"since"` // keys after the mistake; 0 is the mistake
	Keys       int     `json:"keys"`
	MeanFlight int     `json:"meanFlight"`
	Accuracy   float64 `json:"accuracy"`
}

type DoubleStat struct {
	Doubles   int     `json:"doubles"`
	DoubleAcc float64 `json:"doubleAccuracy"`
	DoubleMs  int     `json:"doubleFlight"`
	Singles   int     `json:"singles"`
	SingleAcc float64 `json:"singleAccuracy"`
	SingleMs  int     `json:"singleFlight"`
}

// Insights reads both sections. `runID` of 0 means "no specific run", in which
// case only the lifetime half is populated.
func (s *Store) Insights(typist string, runID int64, limit int) (*Insights, error) {
	out := &Insights{}

	all, err := s.lifetime(typist, limit)
	if err != nil {
		return nil, err
	}
	out.All = all

	if runID != 0 {
		this, err := s.runDetail(runID)
		if err != nil {
			return nil, err
		}
		out.This = this
	}
	return out, nil
}

// runDetail describes a single run, scoped by run_id throughout.
func (s *Store) runDetail(runID int64) (*RunDetail, error) {
	d := &RunDetail{
		ID:       runID,
		Pace:     []PaceBucket{},
		ByWord:   []WordBucket{},
		Misses:   []Confusion{},
		SlowKeys: []KeyStat{},
		Recovery: []RecoveryStep{},
	}

	err := s.db.QueryRow(`
		SELECT COALESCE(wpm,0), COALESCE(accuracy,0),
		       COALESCE(duration_ms,0)/1000.0, COALESCE(keystrokes,0)
		FROM runs WHERE id = ?`, runID,
	).Scan(&d.WPM, &d.Accuracy, &d.Seconds, &d.Keystrokes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("run %d: %w", runID, err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM v_keydowns WHERE run_id = ? AND ok = 0`, runID,
	).Scan(&d.Errors); err != nil {
		return nil, fmt.Errorf("run errors: %w", err)
	}

	if err := s.collect(&d.Pace, `
		SELECT second_bucket, keys, COALESCE(accuracy,0),
		       COALESCE(mean_flight_ms,0), COALESCE(hesitations,0)
		FROM v_run_pace WHERE run_id = ? ORDER BY second_bucket`, runID); err != nil {
		return nil, fmt.Errorf("run pace: %w", err)
	}

	if err := s.collect(&d.ByWord, `
		SELECT word_idx, keys, COALESCE(accuracy,0),
		       COALESCE(mean_flight_ms,0), COALESCE(hesitations,0)
		FROM v_run_words WHERE run_id = ? ORDER BY word_idx`, runID); err != nil {
		return nil, fmt.Errorf("run words: %w", err)
	}

	if err := s.collect(&d.Misses, `
		SELECT expected, COALESCE(typed,''), COALESCE(typed_code,''), times
		FROM v_run_errors WHERE run_id = ?
		ORDER BY times DESC, expected LIMIT 12`, runID); err != nil {
		return nil, fmt.Errorf("run misses: %w", err)
	}

	// Slowest keys in this run, by flight time. Needs at least two attempts so
	// a single unlucky keystroke does not top the list.
	if err := s.collect(&d.SlowKeys, `
		SELECT key, attempts, COALESCE(accuracy,0), COALESCE(mean_flight_ms,0)
		FROM v_run_keys
		WHERE run_id = ? AND key <> ' ' AND attempts >= 2
		ORDER BY mean_flight_ms DESC LIMIT 8`, runID); err != nil {
		return nil, fmt.Errorf("run keys: %w", err)
	}

	if err := s.collect(&d.Recovery, `
		SELECT since_error, keys, COALESCE(mean_flight_ms,0), 0
		FROM v_run_recovery WHERE run_id = ? ORDER BY since_error`, runID); err != nil {
		return nil, fmt.Errorf("run recovery: %w", err)
	}

	var base sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT mean_flight_ms FROM v_run_baseline WHERE run_id = ?`, runID,
	).Scan(&base); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("run baseline: %w", err)
	}
	d.CleanFlight = int(base.Int64)

	return d, nil
}

// lifetime is the across-runs picture.
func (s *Store) lifetime(typist string, limit int) (*Lifetime, error) {
	out := &Lifetime{
		Recent:     []RunSummary{},
		Pace:       []PaceBucket{},
		ByWord:     []WordBucket{},
		Keys:       []KeyStat{},
		Confusions: []Confusion{},
		Recovery:   []RecoveryStep{},
	}

	where, args := "", []any{}
	if typist != "" {
		where, args = " WHERE typist_id = ?", []any{typist}
	}

	// v_own_runs, not runs: a guest's test is stored but is not part of the
	// lifetime picture. Counting it here would also inflate the evidence gate,
	// unlocking claims about "you" on somebody else's keystrokes.
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM v_own_runs`+where, args...).Scan(&out.Runs); err != nil {
		return nil, fmt.Errorf("count runs: %w", err)
	}
	if out.Runs == 0 {
		return out, nil
	}
	// Counted over v_keydowns rather than keystrokes: the base view already
	// excludes guests, auto-repeat and backspaces, so this is the same
	// population every other lifetime number below is computed from.
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM v_keydowns`).Scan(&out.Keystrokes); err != nil {
		return nil, fmt.Errorf("count keystrokes: %w", err)
	}

	if err := s.collect(&out.Recent, `
		SELECT id, started_at, COALESCE(wpm,0), COALESCE(accuracy,0),
		       COALESCE(duration_ms,0)/1000
		FROM v_own_runs`+where+`
		ORDER BY started_at DESC LIMIT ?`,
		append(append([]any{}, args...), limit)...,
	); err != nil {
		return nil, fmt.Errorf("recent: %w", err)
	}

	if err := s.collect(&out.Pace, `
		SELECT second_bucket, keys, COALESCE(accuracy,0),
		       COALESCE(mean_flight_ms,0), COALESCE(hesitations,0)
		FROM v_focus_by_time ORDER BY second_bucket`); err != nil {
		return nil, fmt.Errorf("pace: %w", err)
	}

	if err := s.collect(&out.ByWord, `
		SELECT word_idx, keys, COALESCE(accuracy,0),
		       COALESCE(mean_flight_ms,0), COALESCE(hesitations,0)
		FROM v_focus_by_word ORDER BY word_idx`); err != nil {
		return nil, fmt.Errorf("by word: %w", err)
	}

	if err := s.collect(&out.Keys, `
		SELECT key, attempts, COALESCE(accuracy,0), COALESCE(mean_flight_ms,0)
		FROM v_key_skill WHERE key <> ' ' AND attempts >= 3
		ORDER BY accuracy ASC, mean_flight_ms DESC LIMIT 40`); err != nil {
		return nil, fmt.Errorf("keys: %w", err)
	}

	if err := s.collect(&out.Confusions, `
		SELECT expected, COALESCE(typed,''), COALESCE(typed_code,''), times
		FROM v_confusions ORDER BY times DESC LIMIT 12`); err != nil {
		return nil, fmt.Errorf("confusions: %w", err)
	}

	if err := s.collect(&out.Recovery, `
		SELECT since_error, keys, COALESCE(mean_flight_ms,0), COALESCE(accuracy,0)
		FROM v_error_recovery ORDER BY since_error`); err != nil {
		return nil, fmt.Errorf("recovery: %w", err)
	}

	var base sql.NullInt64
	if err := s.db.QueryRow(`SELECT mean_flight_ms FROM v_clean_baseline`).Scan(&base); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	out.CleanFlight = int(base.Int64)

	out.Doubles = s.doubles()
	return out, nil
}

// doubles compares repeated-letter bigrams against all others.
func (s *Store) doubles() *DoubleStat {
	rows, err := s.db.Query(`
		SELECT is_double, SUM(times), ROUND(AVG(accuracy),1),
		       CAST(AVG(mean_flight_ms) AS INT)
		FROM v_bigrams GROUP BY is_double`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	d := &DoubleStat{}
	for rows.Next() {
		var isDouble, times, flight int
		var acc float64
		if rows.Scan(&isDouble, &times, &acc, &flight) != nil {
			return nil
		}
		if isDouble == 1 {
			d.Doubles, d.DoubleAcc, d.DoubleMs = times, acc, flight
		} else {
			d.Singles, d.SingleAcc, d.SingleMs = times, acc, flight
		}
	}
	if d.Doubles == 0 {
		return nil // not enough evidence to say anything
	}
	return d
}

// collect runs a query and scans every row into a slice of structs, using the
// field order of T. Keeps the query methods above to one line each.
func (s *Store) collect(dst any, query string, args ...any) error {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	switch out := dst.(type) {
	case *[]RunSummary:
		for rows.Next() {
			var v RunSummary
			if err := rows.Scan(&v.ID, &v.StartedAt, &v.WPM, &v.Accuracy, &v.Seconds); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	case *[]PaceBucket:
		for rows.Next() {
			var v PaceBucket
			if err := rows.Scan(&v.Second, &v.Keys, &v.Accuracy, &v.MeanFlight, &v.Hesitations); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	case *[]WordBucket:
		for rows.Next() {
			var v WordBucket
			if err := rows.Scan(&v.Word, &v.Keys, &v.Accuracy, &v.MeanFlight, &v.Hesitations); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	case *[]KeyStat:
		for rows.Next() {
			var v KeyStat
			if err := rows.Scan(&v.Key, &v.Attempts, &v.Accuracy, &v.MeanFlight); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	case *[]Confusion:
		for rows.Next() {
			var v Confusion
			if err := rows.Scan(&v.Expected, &v.Typed, &v.Code, &v.Times); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	case *[]RecoveryStep:
		for rows.Next() {
			var v RecoveryStep
			if err := rows.Scan(&v.Since, &v.Keys, &v.MeanFlight, &v.Accuracy); err != nil {
				return err
			}
			*out = append(*out, v)
		}
	default:
		return fmt.Errorf("collect: unsupported destination %T", dst)
	}
	return rows.Err()
}
