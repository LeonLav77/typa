-- Interpretation lives here, not in the capture path.
--
-- Every query below runs over data that was already collected; none of them
-- required a decision at capture time. That is the point of the design: a new
-- question is a new query, not a new deployment and six months of waiting.
--
-- Timing convention. Two different intervals, never conflated:
--   flight_ms  down-to-down, i.e. previous KEYDOWN to this one. This is the
--              real "how long did that key take" figure, and is what every
--              speed view below uses.
--   hold_ms    dwell, on an 'up' row: how long the key was held.
-- The `gap_ms` column stored on each row is the gap to the previous event of
-- ANY kind (keyups included), so it is NOT flight time. It is kept because it
-- is cheap and occasionally useful, but v_keydowns below is the base view
-- everything else builds on, and it computes flight properly.

-- Which runs count as the typist's own.
--
-- A run tagged `guest` was somebody else at the keyboard. It is stored in full
-- -- it is real data and throwing it away would be a lie about what happened --
-- but it must not reach any view describing "you", or one friend's first
-- attempt drags every per-key average down permanently.
--
-- The exclusion lives HERE, at the base, rather than in each of the thirty
-- views downstream. A filter that has to be remembered in thirty places is a
-- filter that will be forgotten in one, and the failure is silent.
CREATE VIEW IF NOT EXISTS v_own_runs AS
SELECT * FROM runs
WHERE device IS NULL OR device <> 'guest';

-- Base view: character keydowns only, with true down-to-down flight time.
-- Excludes auto-repeat, backspaces (expected IS NULL) and non-key events, so
-- downstream views never have to remember to filter them out. Guest runs are
-- excluded here too, for the same reason.
CREATE VIEW IF NOT EXISTS v_keydowns AS
SELECT k.*,
       k.t_ms - LAG(k.t_ms) OVER (PARTITION BY k.run_id ORDER BY k.seq) AS flight_ms,
       -- Dense position among keydowns only. `seq` counts every event, keyups
       -- included, so consecutive keystrokes are 2 apart in it -- using it to
       -- measure "how many keys since the mistake" would double every offset.
       ROW_NUMBER() OVER (PARTITION BY k.run_id ORDER BY k.seq) AS key_no
FROM (
  SELECT * FROM keystrokes
  WHERE kind = 'down' AND repeat = 0 AND expected IS NOT NULL
    AND run_id IN (SELECT id FROM v_own_runs)
) k;

-- === "You mess up g and h" ==================================================
-- The confusion matrix. Which key did you actually hit when the text wanted
-- something else? Physical `code` is used, not the character, so this stays
-- correct on a split keyboard and across layouts.
CREATE VIEW IF NOT EXISTS v_confusions AS
SELECT expected,
       key                              AS typed,
       code                             AS typed_code,
       COUNT(*)                         AS times,
       COUNT(DISTINCT run_id)           AS runs
FROM v_keydowns
WHERE ok = 0
GROUP BY expected, key, code;

-- === "You are okay with 2-8 but cannot hit 1, 9, 0" ========================
-- Per-key competence: accuracy and typical speed for every key the text asked
-- for. Filter to digits, or letters, or a home row — the shape is the same.
CREATE VIEW IF NOT EXISTS v_key_skill AS
SELECT k.expected                                          AS key,
       COUNT(*)                                            AS attempts,
       SUM(k.ok)                                           AS hits,
       ROUND(100.0 * SUM(k.ok) / COUNT(*), 1)              AS accuracy,
       -- Median is the honest centre here: one 3-second pause would drag a
       -- mean far more than it reflects real ability.
       CAST(AVG(k.flight_ms) AS INT)                       AS mean_flight_ms,
       MIN(k.flight_ms)                                    AS fastest_ms,
       COUNT(DISTINCT k.run_id)                            AS runs
FROM v_keydowns k
GROUP BY k.expected;

-- === "You are good at double letters" ======================================
-- Any bigram, with the repeated-letter case callable out. Self-joined on
-- consecutive keystrokes within a run.
CREATE VIEW IF NOT EXISTS v_bigrams AS
SELECT a.expected || b.expected                            AS bigram,
       (a.expected = b.expected)                           AS is_double,
       COUNT(*)                                            AS times,
       ROUND(100.0 * SUM(b.ok) / COUNT(*), 1)              AS accuracy,
       CAST(AVG(b.flight_ms) AS INT)                       AS mean_flight_ms
FROM v_keydowns a
JOIN v_keydowns b
  ON b.run_id = a.run_id
 AND b.word_idx = a.word_idx          -- within a word, so spaces don't bridge
 AND b.char_pos = a.char_pos + 1      -- strictly consecutive characters
GROUP BY bigram, is_double;

-- === "You lose focus at around X seconds" ==================================
-- Performance bucketed by elapsed time within a run. A rising gap or falling
-- accuracy across buckets is the fatigue curve.
CREATE VIEW IF NOT EXISTS v_focus_by_time AS
SELECT (t_ms / 5000) * 5                                   AS second_bucket,
       COUNT(*)                                            AS keys,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)                AS accuracy,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms,
       SUM(CASE WHEN flight_ms > 500 THEN 1 ELSE 0 END)    AS hesitations
FROM v_keydowns
GROUP BY second_bucket;

-- === "You lose focus at the Nth word" ======================================
-- The same question asked positionally rather than temporally. Both matter:
-- one is fatigue, the other is where in the text attention drops.
CREATE VIEW IF NOT EXISTS v_focus_by_word AS
SELECT word_idx,
       COUNT(*)                                            AS keys,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)                AS accuracy,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms,
       SUM(CASE WHEN flight_ms > 500 THEN 1 ELSE 0 END)    AS hesitations
FROM v_keydowns
GROUP BY word_idx;

-- === Genuine distraction, as opposed to thinking ===========================
-- A long gap that brackets a blur/hide event is the page being left; a long
-- gap with no such event is hesitation. Only the raw stream can tell them
-- apart, which is why focus events are captured at all.
CREATE VIEW IF NOT EXISTS v_away AS
SELECT run_id,
       kind,
       t_ms,
       LEAD(t_ms) OVER (PARTITION BY run_id ORDER BY seq) - t_ms AS away_ms
FROM keystrokes
WHERE kind IN ('blur', 'focus', 'hide', 'show');

-- === Correction behaviour ==================================================
-- How often errors are noticed and fixed, and how fast. Backspaces are logged
-- with expected IS NULL, which is what distinguishes them from character keys.
CREATE VIEW IF NOT EXISTS v_corrections AS
SELECT run_id,
       COUNT(*) FILTER (WHERE key = 'Backspace')           AS backspaces,
       COUNT(*) FILTER (WHERE ok = 0)                      AS errors,
       COUNT(*) FILTER (WHERE expected IS NOT NULL)        AS chars
FROM keystrokes
WHERE kind = 'down' AND repeat = 0
GROUP BY run_id;

-- === Time of day ===========================================================
-- Needs many runs to mean anything, but costs nothing to collect now.
CREATE VIEW IF NOT EXISTS v_by_hour AS
SELECT local_hour,
       COUNT(*)          AS runs,
       CAST(AVG(wpm) AS INT)      AS mean_wpm,
       CAST(AVG(accuracy) AS INT) AS mean_accuracy
FROM v_own_runs
GROUP BY local_hour;

-- === Progress over time ====================================================
-- Per typist, per day. `day` is the typist's LOCAL date: grouping by the UTC
-- date would file an evening run under the previous day for anyone east of
-- Greenwich, which is precisely the population that notices.
CREATE VIEW IF NOT EXISTS v_progress AS
SELECT typist_id,
       DATE((started_at - COALESCE(tz_offset,0) * 60000) / 1000, 'unixepoch') AS day,
       COUNT(*)                             AS runs,
       CAST(AVG(wpm) AS INT)                AS mean_wpm,
       MAX(wpm)                             AS best_wpm,
       CAST(AVG(accuracy) AS INT)           AS mean_accuracy
FROM v_own_runs
GROUP BY typist_id, day;

-- The same by day, across everyone. Used by the history page, which shows all
-- runs rather than one typist's: grouping by typist there produces two rows
-- for one calendar day.
CREATE VIEW IF NOT EXISTS v_days AS
SELECT DATE((started_at - COALESCE(tz_offset,0) * 60000) / 1000, 'unixepoch') AS day,
       COUNT(*)                             AS runs,
       CAST(AVG(wpm) AS INT)                AS mean_wpm,
       MAX(wpm)                             AS best_wpm,
       CAST(AVG(accuracy) AS INT)           AS mean_accuracy
FROM v_own_runs
GROUP BY day;

-- === "You slow down after you make a mistake" ==============================
-- Every keystroke labelled by how far it sits after the most recent error in
-- the same run. Offset 0 is the error itself, 1 is the key right after it, and
-- so on. Comparing mean flight at offsets 1..5 against the run's clean
-- baseline is what turns into "your next 3 keys are 40% slower after a typo".
--
-- The recovery window is bounded to 6 keys so a single early mistake does not
-- label the whole rest of the run as "after an error".
CREATE VIEW IF NOT EXISTS v_after_error AS
WITH marked AS (
  SELECT k.*,
         -- Position of the most recent error at or before this key, counted in
         -- keystrokes (key_no), not raw events.
         MAX(CASE WHEN k.ok = 0 THEN k.key_no END)
           OVER (PARTITION BY k.run_id ORDER BY k.key_no
                 ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS last_err
  FROM v_keydowns k
)
SELECT run_id,
       seq,
       key_no,
       key,
       expected,
       ok,
       flight_ms,
       CASE WHEN last_err IS NULL THEN NULL ELSE key_no - last_err END AS since_error
FROM marked;

-- The headline: typing speed by distance from the last mistake.
-- since_error = 0 is the mistyped key itself; NULL means no error yet.
CREATE VIEW IF NOT EXISTS v_error_recovery AS
SELECT since_error,
       COUNT(*)                                  AS keys,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)      AS accuracy
FROM v_after_error
WHERE since_error IS NOT NULL AND since_error <= 6
GROUP BY since_error;

-- Clean baseline: keys far enough from any error to be considered undisturbed.
CREATE VIEW IF NOT EXISTS v_clean_baseline AS
SELECT COUNT(*)                        AS keys,
       CAST(AVG(flight_ms) AS INT)     AS mean_flight_ms
FROM v_after_error
WHERE since_error IS NULL OR since_error > 6;

-- Do errors cluster? A mistake immediately following another mistake is a
-- different phenomenon (cascade) from an isolated slip.
CREATE VIEW IF NOT EXISTS v_error_clusters AS
SELECT since_error,
       COUNT(*) FILTER (WHERE ok = 0)  AS errors_here,
       COUNT(*)                        AS keys,
       ROUND(100.0 * COUNT(*) FILTER (WHERE ok = 0) / COUNT(*), 1) AS error_rate
FROM v_after_error
WHERE since_error IS NOT NULL AND since_error BETWEEN 1 AND 6
GROUP BY since_error;


-- === Per-run counterparts ==================================================
-- The views above aggregate across every run, which is the right shape for
-- "what are you like as a typist". These keep run_id in the grouping so the
-- same questions can be asked of a single run: "what happened in THIS test".
--
-- Deliberately duplicated rather than parameterised: a view cannot take an
-- argument, and a WHERE clause bolted onto the lifetime views would silently
-- produce per-run numbers under lifetime names.

CREATE VIEW IF NOT EXISTS v_run_pace AS
SELECT run_id,
       (t_ms / 5000) * 5                                   AS second_bucket,
       COUNT(*)                                            AS keys,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)                AS accuracy,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms,
       SUM(CASE WHEN flight_ms > 500 THEN 1 ELSE 0 END)    AS hesitations
FROM v_keydowns
GROUP BY run_id, second_bucket;

CREATE VIEW IF NOT EXISTS v_run_words AS
SELECT run_id,
       word_idx,
       COUNT(*)                                            AS keys,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)                AS accuracy,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms,
       SUM(CASE WHEN flight_ms > 500 THEN 1 ELSE 0 END)    AS hesitations
FROM v_keydowns
GROUP BY run_id, word_idx;

CREATE VIEW IF NOT EXISTS v_run_errors AS
SELECT run_id,
       expected,
       key                                                 AS typed,
       code                                                AS typed_code,
       COUNT(*)                                            AS times
FROM v_keydowns
WHERE ok = 0
GROUP BY run_id, expected, key, code;

CREATE VIEW IF NOT EXISTS v_run_recovery AS
SELECT run_id,
       since_error,
       COUNT(*)                                            AS keys,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms
FROM v_after_error
WHERE since_error IS NOT NULL AND since_error <= 6
GROUP BY run_id, since_error;

CREATE VIEW IF NOT EXISTS v_run_baseline AS
SELECT run_id,
       COUNT(*)                                            AS keys,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms
FROM v_after_error
WHERE since_error IS NULL OR since_error > 6
GROUP BY run_id;

-- Slowest keys within one run, for the "this run" panel.
CREATE VIEW IF NOT EXISTS v_run_keys AS
SELECT run_id,
       expected                                            AS key,
       COUNT(*)                                            AS attempts,
       SUM(ok)                                             AS hits,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)                AS accuracy,
       CAST(AVG(flight_ms) AS INT)                         AS mean_flight_ms
FROM v_keydowns
GROUP BY run_id, expected;

-- ===========================================================================
-- Deep analytics. Everything below joins the raw stream to physical geometry,
-- so questions about hands, fingers and rows become answerable without any
-- change to what is captured.
-- ===========================================================================

-- Keydowns enriched with the physical geometry of the key that was WANTED.
-- Joining on `expected` rather than what was typed keeps the question "how
-- good is your left index finger" about the finger that should have moved.
CREATE VIEW IF NOT EXISTS v_keys_geo AS
SELECT k.*,
       g.finger, g.hand, g.row
FROM v_keydowns k
LEFT JOIN key_geometry g ON g.code = k.code;

-- Per finger.
CREATE VIEW IF NOT EXISTS v_finger AS
SELECT finger, hand,
       COUNT(*)                                  AS attempts,
       SUM(ok)                                   AS hits,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)      AS accuracy,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms
FROM v_keys_geo
WHERE finger IS NOT NULL
GROUP BY finger, hand;

-- Per hand.
CREATE VIEW IF NOT EXISTS v_hand AS
SELECT hand,
       COUNT(*)                                  AS attempts,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)      AS accuracy,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms
FROM v_keys_geo
WHERE hand IS NOT NULL AND hand <> 'either'
GROUP BY hand;

-- Per row. Reaching to the number row is measurably slower than home row for
-- most people; this is where that shows up.
CREATE VIEW IF NOT EXISTS v_row AS
SELECT row,
       COUNT(*)                                  AS attempts,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)      AS accuracy,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms
FROM v_keys_geo
WHERE row IS NOT NULL
GROUP BY row;

-- Hand alternation. Typing that alternates hands is faster than same-hand
-- runs, and same-FINGER bigrams are the slowest of all -- the classic layout
-- pain point, and the main thing a split keyboard user tunes for.
CREATE VIEW IF NOT EXISTS v_transitions AS
SELECT CASE
         WHEN a.hand IS NULL OR b.hand IS NULL THEN 'unknown'
         WHEN a.finger = b.finger THEN 'same finger'
         WHEN a.hand = b.hand THEN 'same hand'
         ELSE 'alternating'
       END                                       AS pattern,
       COUNT(*)                                  AS times,
       CAST(AVG(b.flight_ms) AS INT)             AS mean_flight_ms,
       ROUND(100.0 * SUM(b.ok) / COUNT(*), 1)    AS accuracy
FROM v_keys_geo a
JOIN v_keys_geo b
  ON b.run_id = a.run_id AND b.key_no = a.key_no + 1
WHERE b.flight_ms IS NOT NULL
GROUP BY pattern;

-- Row jumps: moving between rows costs time.
CREATE VIEW IF NOT EXISTS v_row_jumps AS
SELECT a.row || ' to ' || b.row                  AS move,
       COUNT(*)                                  AS times,
       CAST(AVG(b.flight_ms) AS INT)             AS mean_flight_ms,
       ROUND(100.0 * SUM(b.ok) / COUNT(*), 1)    AS accuracy
FROM v_keys_geo a
JOIN v_keys_geo b
  ON b.run_id = a.run_id AND b.key_no = a.key_no + 1
WHERE a.row IS NOT NULL AND b.row IS NOT NULL AND b.flight_ms IS NOT NULL
GROUP BY move;

-- Where in a word errors happen. First letters and interiors fail differently:
-- a first-letter error is usually a word-recognition problem, an interior one
-- is motor.
CREATE VIEW IF NOT EXISTS v_error_position AS
SELECT CASE WHEN char_pos = 0 THEN 'first letter' ELSE 'later letters' END AS place,
       COUNT(*)                                  AS attempts,
       SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END)   AS errors,
       ROUND(100.0 * SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END) / COUNT(*), 2) AS error_rate,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms
FROM v_keydowns
GROUP BY place;

-- Speed by how long the word is. Long words are not simply slower per key --
-- often the opposite, since common long words are typed as one motion.
CREATE VIEW IF NOT EXISTS v_word_length AS
SELECT LENGTH(w.word)                            AS length,
       COUNT(DISTINCT w.run_id || ':' || w.idx)  AS words,
       ROUND(100.0 * SUM(w.correct) / COUNT(DISTINCT w.run_id || ':' || w.idx), 1) AS pct_clean,
       CAST(AVG(k.flight_ms) AS INT)             AS mean_flight_ms
FROM run_words w
JOIN v_keydowns k ON k.run_id = w.run_id AND k.word_idx = w.idx
GROUP BY length
HAVING words >= 3;

-- The words you get wrong most often.
CREATE VIEW IF NOT EXISTS v_hard_words AS
SELECT word,
       COUNT(*)                                  AS seen,
       SUM(correct)                              AS clean,
       ROUND(100.0 * SUM(correct) / COUNT(*), 1) AS pct_clean
FROM run_words
GROUP BY word
HAVING seen >= 3;

-- Rhythm. Consistency of keystroke timing is a better measure of mastery than
-- raw speed: a steady 60wpm beats a spiky 80wpm for accuracy.
CREATE VIEW IF NOT EXISTS v_rhythm AS
SELECT run_id,
       COUNT(*)                                  AS keys,
       CAST(AVG(flight_ms) AS INT)               AS mean_flight_ms,
       -- Population standard deviation, computed longhand: SQLite has no
       -- stddev() built in.
       CAST(SQRT(AVG(flight_ms * flight_ms) - AVG(flight_ms) * AVG(flight_ms)) AS INT) AS sd_ms,
       MIN(flight_ms)                            AS fastest_ms,
       MAX(flight_ms)                            AS slowest_ms
FROM v_keydowns
WHERE flight_ms IS NOT NULL AND flight_ms < 2000
GROUP BY run_id;

-- Pause taxonomy. Short gaps are motor noise; long ones are decisions.
CREATE VIEW IF NOT EXISTS v_pauses AS
SELECT CASE
         WHEN flight_ms < 200  THEN 'flow (<200ms)'
         WHEN flight_ms < 400  THEN 'steady (200-400ms)'
         WHEN flight_ms < 800  THEN 'thinking (400-800ms)'
         WHEN flight_ms < 2000 THEN 'stalled (0.8-2s)'
         ELSE 'stopped (>2s)'
       END                                       AS band,
       COUNT(*)                                  AS times,
       ROUND(100.0 * SUM(ok) / COUNT(*), 1)      AS accuracy
FROM v_keydowns
WHERE flight_ms IS NOT NULL
GROUP BY band;

-- Dwell: how long keys are held. Distinct from flight, and a different signal.
CREATE VIEW IF NOT EXISTS v_dwell AS
SELECT k.key,
       COUNT(*)                                  AS presses,
       CAST(AVG(k.hold_ms) AS INT)               AS mean_hold_ms
FROM keystrokes k
WHERE k.kind = 'up' AND k.hold_ms IS NOT NULL AND k.hold_ms < 1000
GROUP BY k.key;

-- Day-of-week and hour, for "when do you type best".
CREATE VIEW IF NOT EXISTS v_by_dow AS
SELECT local_dow,
       COUNT(*)                                  AS runs,
       CAST(AVG(wpm) AS INT)                     AS mean_wpm,
       CAST(AVG(accuracy) AS INT)                AS mean_accuracy
FROM v_own_runs
GROUP BY local_dow;

-- Trigrams: three-key sequences, where real motor patterns live.
CREATE VIEW IF NOT EXISTS v_trigrams AS
SELECT a.expected || b.expected || c.expected     AS trigram,
       COUNT(*)                                   AS times,
       CAST(AVG(b.flight_ms + c.flight_ms) AS INT) AS mean_span_ms,
       ROUND(100.0 * SUM(CASE WHEN b.ok = 1 AND c.ok = 1 THEN 1 ELSE 0 END) / COUNT(*), 1) AS accuracy
FROM v_keydowns a
JOIN v_keydowns b ON b.run_id = a.run_id AND b.word_idx = a.word_idx AND b.char_pos = a.char_pos + 1
JOIN v_keydowns c ON c.run_id = a.run_id AND c.word_idx = a.word_idx AND c.char_pos = a.char_pos + 2
WHERE b.flight_ms IS NOT NULL AND c.flight_ms IS NOT NULL
GROUP BY trigram
HAVING times >= 3;

-- === Comparing setups ======================================================
-- The point of tagging every run: "am I actually faster on the Moonlander?"
-- is a question about two populations, and it can only be asked because the
-- tag was recorded at capture time rather than guessed afterwards.
--
-- Guests are excluded by v_own_runs, so these compare YOUR hands on different
-- keyboards, not your hands against someone else's.

-- Headline numbers per keyboard.
CREATE VIEW IF NOT EXISTS v_by_device AS
SELECT COALESCE(device, 'untagged')       AS device,
       COUNT(*)                           AS runs,
       CAST(AVG(wpm) AS INT)              AS mean_wpm,
       MAX(wpm)                           AS best_wpm,
       CAST(AVG(accuracy) AS INT)         AS mean_accuracy
FROM v_own_runs
GROUP BY device;

-- Headline numbers per place.
CREATE VIEW IF NOT EXISTS v_by_location AS
SELECT COALESCE(location, 'untagged')     AS location,
       COUNT(*)                           AS runs,
       CAST(AVG(wpm) AS INT)              AS mean_wpm,
       MAX(wpm)                           AS best_wpm,
       CAST(AVG(accuracy) AS INT)         AS mean_accuracy
FROM v_own_runs
GROUP BY location;

-- Both at once: the cell you actually type in.
CREATE VIEW IF NOT EXISTS v_by_setup AS
SELECT COALESCE(location, 'untagged') || ' · ' || COALESCE(device, 'untagged') AS setup,
       COUNT(*)                           AS runs,
       CAST(AVG(wpm) AS INT)              AS mean_wpm,
       CAST(AVG(accuracy) AS INT)         AS mean_accuracy
FROM v_own_runs
GROUP BY location, device;

-- Per-key pace split by keyboard. This is the one that answers "which keys is
-- the Moonlander actually helping with", rather than only the headline speed.
CREATE VIEW IF NOT EXISTS v_key_by_device AS
SELECT COALESCE(r.device, 'untagged')     AS device,
       k.expected                         AS key,
       COUNT(*)                           AS attempts,
       ROUND(100.0 * SUM(k.ok) / COUNT(*), 1) AS accuracy,
       CAST(AVG(k.flight_ms) AS INT)      AS mean_flight_ms
FROM v_keydowns k
JOIN v_own_runs r ON r.id = k.run_id
GROUP BY r.device, k.expected;

-- Per finger, split by keyboard: a split board changes which finger owns which
-- key, so this is where a layout change shows up if it shows up anywhere.
CREATE VIEW IF NOT EXISTS v_finger_by_device AS
SELECT COALESCE(r.device, 'untagged')     AS device,
       g.finger, g.hand,
       COUNT(*)                           AS attempts,
       ROUND(100.0 * SUM(k.ok) / COUNT(*), 1) AS accuracy,
       CAST(AVG(k.flight_ms) AS INT)      AS mean_flight_ms
FROM v_keydowns k
JOIN v_own_runs r ON r.id = k.run_id
LEFT JOIN key_geometry g ON g.code = k.code
WHERE g.finger IS NOT NULL
GROUP BY r.device, g.finger, g.hand;
