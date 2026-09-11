-- Raw capture schema.
--
-- Design rule: this stores what happened, never what it means. There are no
-- "hesitation" or "confusion" columns, because those are definitions, and
-- definitions change. Every insight -- focus decay, g/h confusion, weak
-- digits, double-letter skill -- is a query over `keystrokes`, so a new
-- question can be asked of data collected before the question existed.
--
-- One row per key event is deliberate. A JSON blob per run would be smaller
-- but would make the interesting queries (per-key, per-bigram, positional)
-- either impossible or unbearably slow.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- A typist. Anonymous by default: an id minted in the browser, no account.
CREATE TABLE IF NOT EXISTS typists (
  id          TEXT PRIMARY KEY,
  first_seen  INTEGER NOT NULL,          -- unix ms
  last_seen   INTEGER NOT NULL
);

-- One completed test.
CREATE TABLE IF NOT EXISTS runs (
  id            INTEGER PRIMARY KEY,
  test_id       TEXT    NOT NULL,        -- the id the server dealt
  typist_id     TEXT    REFERENCES typists(id),
  schema_v      INTEGER NOT NULL,        -- payload version, for migrations

  -- Clocks. started_at locates the run in real time (time-of-day, streaks);
  -- event offsets are monotonic and immune to clock changes.
  started_at    INTEGER NOT NULL,        -- unix ms, first keystroke
  ended_at      INTEGER NOT NULL,
  duration_ms   INTEGER NOT NULL,

  -- Local wall-clock parts, denormalised so "worse late at night" and
  -- "weekends are slower" are cheap GROUP BYs rather than per-row math.
  local_hour    INTEGER,                 -- 0-23 in the typist's timezone
  local_dow     INTEGER,                 -- 0=Sunday
  tz            TEXT,
  tz_offset     INTEGER,                 -- minutes

  -- Headline stats. Recomputable from keystrokes; kept for fast listing only.
  wpm           INTEGER,
  raw_wpm       INTEGER,
  accuracy      INTEGER,
  words         INTEGER,
  words_right   INTEGER,
  chars_typed   INTEGER,
  chars_correct INTEGER,
  keystrokes    INTEGER,

  -- Context to control for when comparing runs.
  --
  -- Two tags that decide whether a run belongs in "your" numbers at all.
  -- Both are stored as plain strings rather than a constrained enum: a new
  -- place or a borrowed keyboard should never be a schema migration.
  mode          TEXT,                    -- what the text was made of: 'words+numbers'
  location      TEXT,                    -- 'work' | 'home' | 'other' | NULL
  location_src  TEXT,                    -- how it was decided: 'geo' | 'manual' | NULL
  device        TEXT,                    -- 'moonlander' | 'normal' | 'guest' | NULL
  device_src    TEXT,                    -- 'hid' | 'manual' | 'default' | NULL
  layout        TEXT,                    -- e.g. 'split-qwerty'
  ua            TEXT,
  platform      TEXT,
  viewport_w    INTEGER,
  viewport_h    INTEGER,
  cores         INTEGER,
  memory        REAL,

  received_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS runs_typist_time ON runs(typist_id, started_at);
CREATE INDEX IF NOT EXISTS runs_hour        ON runs(local_hour);

-- The words a run dealt, and what was typed into each. Per-word timing lives
-- here so "you slow down at the 7th word" is a direct aggregate.
CREATE TABLE IF NOT EXISTS run_words (
  run_id      INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  idx         INTEGER NOT NULL,          -- position in the run, 0-based
  word        TEXT    NOT NULL,
  typed       TEXT,
  correct     INTEGER NOT NULL,          -- 0/1, typed == word
  entered_ms  INTEGER,                   -- offset when the word became active
  first_ms    INTEGER,                   -- first keystroke on it
  last_ms     INTEGER,                   -- last keystroke on it
  left_ms     INTEGER,                   -- offset when committed
  PRIMARY KEY (run_id, idx)
);

-- Every key event, verbatim. The heart of the whole design.
CREATE TABLE IF NOT EXISTS keystrokes (
  run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq        INTEGER NOT NULL,           -- order within the run
  kind       TEXT    NOT NULL,           -- down | up | blur | focus | hide | show | resize
  t_ms       INTEGER NOT NULL,           -- offset from run start

  key        TEXT,                       -- character produced ('g')
  code       TEXT,                       -- PHYSICAL key ('KeyG') -- layout-independent
  expected   TEXT,                       -- what the text wanted here, NULL if none
  ok         INTEGER,                    -- 1 hit, 0 miss, NULL n/a
  word_idx   INTEGER,                    -- which word
  char_pos   INTEGER,                    -- position within that word
  mods       INTEGER,                    -- shift|ctrl|alt|meta bits
  repeat     INTEGER,                    -- auto-repeat from a held key
  hold_ms    INTEGER,                    -- dwell, on 'up' events

  -- Gap since the previous event. Stored because nearly every timing question
  -- starts here, and a window function per query is wasteful at scale.
  gap_ms     INTEGER,

  PRIMARY KEY (run_id, seq)
);

CREATE INDEX IF NOT EXISTS ks_expected ON keystrokes(expected, ok);
CREATE INDEX IF NOT EXISTS ks_code     ON keystrokes(code);
CREATE INDEX IF NOT EXISTS ks_run_seq  ON keystrokes(run_id, seq);

-- Physical key geometry: which finger, hand and row owns each key code.
-- Rewritten from keymap.go on every start, so it is derived data, never a
-- source of truth. Kept in SQL only so analysis can JOIN rather than repeating
-- a large CASE expression in every query.
CREATE TABLE IF NOT EXISTS key_geometry (
  code    TEXT PRIMARY KEY,
  finger  TEXT NOT NULL,
  hand    TEXT NOT NULL,
  row     TEXT NOT NULL
);
