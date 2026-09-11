/**
 * Pure typing state. No DOM, no word list — the server deals the words and
 * this tracks what happens to them. Every keystroke is appended to `events`
 * so a later version can ship a full replay back to the server.
 */
export function createTest(words, id = null) {
  return {
    id,                         // server handle for the dealt test
    words,
    typed: words.map(() => ''), // what the user actually entered per word
    index: 0,                   // active word
    startedAt: null,            // ms epoch of first keystroke
    endedAt: null,
    events: [],                 // { t, key, expected, word, pos, ok }
  };
}

export const isRunning = (t) => t.startedAt !== null && t.endedAt === null;
export const isDone = (t) => t.endedAt !== null;

function log(t, key, expected, ok) {
  t.events.push({
    t: Math.round(performance.now() - t.startMark),
    key,
    expected,
    word: t.index,
    pos: t.typed[t.index].length,
    ok,
  });
}

function start(t) {
  if (t.startedAt === null) {
    t.startedAt = Date.now();
    t.startMark = performance.now();
  }
}

/** Type one character into the active word. Returns true if state changed. */
export function press(t, key) {
  if (isDone(t)) return false;
  start(t);
  const word = t.words[t.index];
  const pos = t.typed[t.index].length;
  const expected = pos < word.length ? word[pos] : null;
  log(t, key, expected, key === expected);
  t.typed[t.index] += key;
  // Last word typed to full length ends the test without needing a space.
  if (t.index === t.words.length - 1 && t.typed[t.index].length >= word.length) finish(t);
  return true;
}

/** Space: commit the word as-is and move on, right or wrong. */
export function commit(t) {
  if (isDone(t)) return false;
  if (t.startedAt === null) return false; // no leading space
  start(t);
  log(t, ' ', ' ', true);
  if (t.index === t.words.length - 1) {
    finish(t);
  } else {
    t.index++;
  }
  return true;
}

/** Backspace inside the current word only — committed words stay committed. */
export function backspace(t, whole = false) {
  if (isDone(t)) return false;
  const cur = t.typed[t.index];
  if (cur === '') return false;
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
  const keys = t.events.filter((e) => e.key !== ' ');
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

/** Shape a finished test for the future /api/results endpoint. */
export function toResult(t) {
  return {
    v: 1,
    id: t.id,
    startedAt: t.startedAt,
    endedAt: t.endedAt,
    words: t.words,
    typed: t.typed,
    events: t.events,
    stats: stats(t),
  };
}
