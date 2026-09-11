import { createRecorder, begin, mark, keyDown, keyUp, note, environment, KIND } from './capture.js';

/**
 * Pure typing state. No DOM, no word list — the server deals the words and
 * this tracks what happens to them. The raw event stream lives on `rec` (see
 * capture.js) and is shipped verbatim to the server, because every later
 * insight is a query over it rather than a number computed here.
 */
export function createTest(words, id = null, mode = '') {
  return {
    id,                         // server handle for the dealt test
    mode,                       // what the server was asked to generate
    words,
    typed: words.map(() => ''), // what the user actually entered per word
    index: 0,                   // active word
    startedAt: null,            // ms epoch of first keystroke
    endedAt: null,
    rec: createRecorder(),      // raw capture: keystrokes, focus, environment
    // Per-word timing, recorded as it happens. Derivable from the event
    // stream, but cheap here and it makes "slow on the 7th word" a one-line
    // query instead of a replay.
    wordTimes: words.map(() => ({ first: null, last: null, entered: null, left: null })),
  };
}

export const isRunning = (t) => t.startedAt !== null && t.endedAt === null;
export const isDone = (t) => t.endedAt !== null;

/** Record a keystroke against the current position in the text. */
function log(t, e, expected, ok) {
  const ev = keyDown(t.rec, e, {
    word: t.index,
    pos: t.typed[t.index].length,
    expected,
    ok,
  });
  const wt = t.wordTimes[t.index];
  if (wt.first === null) wt.first = ev.t;
  wt.last = ev.t;
  return ev;
}

function start(t) {
  if (t.startedAt === null) {
    begin(t.rec);
    t.startedAt = t.rec.wallStart;
    t.startMark = t.rec.monoStart;
    // The first word is entered the moment the clock starts.
    t.wordTimes[t.index].entered = 0;
  }
}

/**
 * Type one character into the active word. `e` is the raw KeyboardEvent, so
 * the physical key code and modifier state are captured alongside the
 * character. Returns true if state changed.
 */
export function press(t, e) {
  if (isDone(t)) return false;
  const key = e.key;
  start(t);
  const word = t.words[t.index];
  const pos = t.typed[t.index].length;
  const expected = pos < word.length ? word[pos] : null;
  log(t, e, expected, key === expected);
  t.typed[t.index] += key;
  // Last word typed to full length ends the test without needing a space.
  if (t.index === t.words.length - 1 && t.typed[t.index].length >= word.length) finish(t);
  return true;
}

/** Space: commit the word as-is and move on, right or wrong. */
export function commit(t, e) {
  if (isDone(t)) return false;
  if (t.startedAt === null) return false; // no leading space
  start(t);
  const ev = log(t, e, ' ', true);
  t.wordTimes[t.index].left = ev.t;
  if (t.index === t.words.length - 1) {
    finish(t);
  } else {
    t.index++;
    t.wordTimes[t.index].entered = ev.t;
  }
  return true;
}

/**
 * Backspace inside the current word only — committed words stay committed.
 * Corrections are logged like any other key: how often and how quickly a user
 * corrects is one of the stronger signals in the stream.
 */
export function backspace(t, e, whole = false) {
  if (isDone(t)) return false;
  const cur = t.typed[t.index];
  if (cur === '') return false;
  start(t);
  log(t, e, null, true);
  t.typed[t.index] = whole ? '' : cur.slice(0, -1);
  return true;
}

export function finish(t) {
  if (t.endedAt === null) t.endedAt = Date.now();
}

/** Standard WPM: correct characters (plus one space per completed word) / 5. */
export function stats(t) {
  const end = t.endedAt ?? Date.now();
  const seconds = t.startedAt ? Math.max((end - t.startedAt) / 1000, 0.001) : 0;
  const minutes = seconds / 60;

  let correct = 0, typedChars = 0, wordsRight = 0;
  const last = isDone(t) ? t.words.length - 1 : t.index;
  for (let i = 0; i <= last; i++) {
    const word = t.words[i], got = t.typed[i];
    typedChars += got.length;
    let hits = 0;
    for (let c = 0; c < got.length; c++) if (got[c] === word[c]) hits++;
    correct += hits;
    if (got === word) {
      wordsRight++;
      if (i < last || isDone(t)) correct++; // the space after a correct word counts
    }
  }
  // Accuracy counts character keydowns only: not spaces, not backspaces
  // (expected === null), and not auto-repeat from a held key.
  const keys = t.rec.events.filter(
    (e) => e.k === KIND.DOWN && !e.rep && e.key !== ' ' && e.expected !== null,
  );
  const hitKeys = keys.filter((e) => e.ok).length;

  return {
    seconds,
    wpm: minutes ? Math.round(correct / 5 / minutes) : 0,
    raw: minutes ? Math.round(typedChars / 5 / minutes) : 0,
    accuracy: keys.length ? Math.round((hitKeys / keys.length) * 100) : 100,
    correct,
    typedChars,
    wordsRight,
    words: t.words.length,
    keystrokes: keys.length,
  };
}

/**
 * Package a finished run for POST /api/results.
 *
 * This ships the RAW stream, not conclusions. `stats` is included only because
 * it is cheap and useful for listing runs; every one of its numbers is
 * recomputable from `events`, and analysis should prefer the events. The
 * layout tag travels with the run so that a later change of keyboard does not
 * silently pollute historical per-finger analysis.
 */
export function toResult(t, layout = null, context = {}) {
  return {
    v: 2,
    id: t.id,
    startedAt: t.startedAt,     // wall clock: time-of-day analysis
    endedAt: t.endedAt,
    monoStart: t.rec.monoStart, // origin for every event offset
    words: t.words,
    typed: t.typed,
    wordTimes: t.wordTimes,
    events: t.rec.events,       // the whole truth; everything derives from here
    env: environment(),
    layout,
    // The mode the server generated this text to. Stored so "slower with
    // punctuation" is a question about a tag rather than a re-parse of the
    // words after the fact.
    mode: t.mode || '',
    // Where and on what. Resolved at the end of the run rather than the start,
    // so unplugging a keyboard mid-test tags the run by what finished it.
    location: context.location || '',
    locationSrc: context.locationSrc || '',
    device: context.device || '',
    deviceSrc: context.deviceSrc || '',
    stats: stats(t),
  };
}
